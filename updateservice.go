package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// 更新流程事件（updater 包生命周期事件的 project 内翻译，前端订阅这些名字）。
const (
	updateAvailableEvent = "update:available"
	updateLatestEvent    = "update:latest"
	updateProgressEvent  = "update:progress"
	updateReadyEvent     = "update:ready"
	updateErrorEvent     = "update:error"
)

// updateCheckInterval 是后台自动检查更新的间隔。
const updateCheckInterval = 6 * time.Hour

// updateRepository 是发布 Release 的 GitHub 仓库。
const updateRepository = "younland/RcloneDrive"

// updateHTTPTimeout 是更新相关 HTTP 请求的总超时。便携版二进制近 80MB，
// 默认 30s 总超时会掐断慢速网络下的下载；完整性靠 release 附带的 checksums 校验兜底。
const updateHTTPTimeout = 30 * time.Minute

// updateTransport 构建更新器的 HTTP Transport：优先走系统代理。
// Go 的 HTTP 客户端默认不读系统代理（浏览器走代理），直连 github.com 被
// 网络阻断时检查更新会超时；无系统代理时维持默认行为（读 HTTPS_PROXY 环境变量）。
// 各平台系统代理来源见 proxy_*.go：Windows 注册表、macOS scutil、Linux gsettings/环境变量。
func updateTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if u := systemProxy(); u != nil {
		transport.Proxy = http.ProxyURL(u)
	}
	return transport
}

// ensureReleaseDigest 为发现的新版本取回并挂上 SHA-256 摘要。
// Release 的校验文件按发行包名区分（<包名>.sha256），名字随版本变化，
// 只能在 Check 之后按本平台的发行包名取回；注入 rel.Verification 后
// updater 下载时会强制校验，下载被截断（提前 EOF）或文件被替换都会被拦住。
func (s *UpdateService) ensureReleaseDigest(rel *updater.Release) error {
	if rel.Verification != nil && len(rel.Verification.Digest) > 0 {
		return nil
	}
	assetURL, _ := rel.Metadata["github.asset.url"].(string)
	if assetURL == "" {
		return errors.New("缺少资产下载地址，无法获取校验信息")
	}
	digest, err := fetchSidecarDigest(assetURL+".sha256", rel.Artifact.Filename)
	if err != nil {
		return err
	}
	rel.Verification = &updater.Verification{Digest: digest, DigestAlgo: "sha256"}
	return nil
}

// fetchSidecarDigest 下载 <发行包名>.sha256 并解析出摘要。
// 文件内容为 sha256sum 标准格式「<哈希>  <文件名>」，也兼容只有哈希的单行形式；
// 带文件名时核对必须与目标发行包一致，防止张冠李戴。
func fetchSidecarDigest(urlStr, wantName string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: updateHTTPTimeout, Transport: updateTransport()}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载校验文件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载校验文件失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return nil, errors.New("校验文件内容为空")
	}
	if len(fields) >= 2 && fields[1] != wantName {
		return nil, fmt.Errorf("校验文件对应的发行包不符：%s ≠ %s", fields[1], wantName)
	}
	digest, err := hex.DecodeString(fields[0])
	if err != nil || len(digest) != sha256.Size {
		return nil, errors.New("校验文件中的摘要不合法")
	}
	return digest, nil
}

// UpdateStatus 描述当前更新状态，随 update:* 事件推给前端，也是 Status() 的返回。
type UpdateStatus struct {
	Current    string `json:"current"`
	State      string `json:"state"` // updater.State：idle/checking/available/downloading/ready/error 等
	Latest     string `json:"latest,omitempty"`
	Notes      string `json:"notes,omitempty"`
	URL        string `json:"url,omitempty"`
	Downloaded bool   `json:"downloaded"`
	// Verified 为 true 表示已下载的二进制与 release 公布的 SHA-256 二次校验一致，
	// 此时才允许重启生效；false 时「立即重启」不可用。
	Verified bool `json:"verified"`
}

// UpdateErrorEvent 是 update:error 事件的负载。
type UpdateErrorEvent struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// UpdateService 封装 Wails updater：GitHub Releases 为更新源，
// 发现新版本后自动下载安装，重启由用户确认（见事件处理）。
// 自动检查默认关闭，仅在用户在设置中开启后才启动；手动「检查更新」始终可用。
type UpdateService struct {
	app      *application.App
	settings *SettingsService

	mu       sync.Mutex
	release  *updater.Release // 最近一次发现的新版本
	progress updater.Progress // 最近一次下载进度
	// stagedInstaller 是安装版已下载并校验通过的 NSIS 安装包路径；
	// 非空时 Restart 走「退出 → 静默安装 → 重启应用」而不是二进制替换。
	stagedInstaller string

	// autoStarted 防止重复自动下载（周期检查与手动检查可能同时命中可用事件）。
	autoStarted atomic.Bool
	// verified 记录最近一次下载完成后的二次校验结果，Restart 据此放行。
	verified atomic.Bool
	// toastTarget 记录最近一次检查的发起窗口，后续事件提示按它定向
	// （设置窗口发起的检查，提示应出现在设置窗口而不是主窗口）。
	toastTarget atomic.Value
}

// NewUpdateService 创建更新服务。
func NewUpdateService(settings *SettingsService) *UpdateService {
	return &UpdateService{settings: settings}
}

// ServiceStartup 初始化 updater 并订阅其生命周期事件。
// 开发构建不初始化 updater（见 autoupdate_dev.go），不做任何自动/手动更新。
func (s *UpdateService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	if !autoUpdateEnabled {
		log.Printf("开发构建：自动更新已禁用")
		return nil
	}

	s.app = application.Get()
	if s.app == nil {
		return nil
	}

	gh, err := github.New(github.Config{
		Repository:   updateRepository,
		AssetMatcher: updateAssetMatcher,
		// 不用提供商的 ChecksumAsset：按发行包名区分的校验文件（<包名>.sha256）
		// 名字随版本变化，初始化时还不知道；改为 Check 之后自行取回摘要并
		// 注入 rel.Verification（见 ensureReleaseDigest），updater 下载时照样
		// 强制校验（fail closed）。
		HTTPClient: &http.Client{Timeout: updateHTTPTimeout, Transport: updateTransport()},
	})
	if err != nil {
		log.Printf("更新器初始化失败: %v", err)
		return nil // 更新是增强功能，不阻断启动
	}

	err = s.app.Updater.Init(updater.Config{
		CurrentVersion: appVersion,
		Providers:      []updater.Provider{gh},
		// CheckInterval 置 0：周期检查由本服务按「自动更新」设置自行调度，
		// 设置关闭时不做任何网络检查。
		CheckInterval: 0,
	})
	if err != nil {
		log.Printf("更新器初始化失败: %v", err)
		return nil
	}

	s.subscribe()
	s.sweepStaleUpdateArtifacts()
	go s.autoCheckLoop()
	return nil
}

// sweepStaleUpdateArtifacts 清理上一次二进制替换升级遗留的 <exe>.old.* 文件。
// Wails 的替换助手在映像仍被内核持有时删不掉它们，只约定「下次升级时顺手清」，
// 导致文件无限期残留；应用重启后旧映像必然已释放，这里兜底删除。
func (s *UpdateService) sweepStaleUpdateArtifacts() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	matches, err := filepath.Glob(exe + ".old.*")
	if err != nil {
		return
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			log.Printf("清理更新残留失败 %s: %v", m, err)
		}
	}
}

// autoCheckLoop 按固定周期复查更新；每次 tick 重新读设置，
// 用户在设置里开关自动更新后无需重启即生效。
func (s *UpdateService) autoCheckLoop() {
	// 启动后稍等片刻做第一次检查（不拖慢启动，也让网络就绪）；后台检查保持静默
	time.Sleep(10 * time.Second)
	if s.settings.AutoUpdate() {
		s.checkBackground()
	}
	ticker := time.NewTicker(updateCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		if s.settings.AutoUpdate() {
			s.checkBackground()
		}
	}
}

// checkBackground 静默检查：有更新才通知，已是最新不打扰。
func (s *UpdateService) checkBackground() {
	if s.app == nil || s.app.Updater == nil {
		return
	}
	s.toastTarget.Store("main")
	if _, err := s.app.Updater.Check(context.Background()); err != nil {
		log.Printf("检查更新失败: %v", err)
	}
}

// currentToastTarget 返回最近一次检查发起方的窗口名（缺省主窗口）。
func (s *UpdateService) currentToastTarget() string {
	if v, ok := s.toastTarget.Load().(string); ok && v != "" {
		return v
	}
	return "main"
}

// subscribe 把 updater 的生命周期事件翻译成 project 内事件与通知。
func (s *UpdateService) subscribe() {
	target := s.currentToastTarget
	s.app.Event.On(updater.EventUpdateAvailable, func(e *application.CustomEvent) {
		rel, _ := e.Data.(*updater.Release)
		if rel == nil {
			return
		}
		// 先取回本发行包的校验摘要：取不到就不允许下载，避免装上无法验证来源的更新
		if err := s.ensureReleaseDigest(rel); err != nil {
			s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "verify", Message: err.Error()})
			notifyUserTo(target(), "error", "获取更新校验信息失败", err.Error())
			log.Printf("获取更新校验信息失败: %v", err)
			return
		}
		s.mu.Lock()
		s.release = rel
		s.progress = updater.Progress{Total: rel.Artifact.Size}
		s.mu.Unlock()
		s.verified.Store(false) // 新版本待下载，校验结果作废

		s.emitStatus(updateAvailableEvent)
		notifyUserTo(target(), "success", "发现新版本 v"+rel.Version, "开始自动下载，完成后重启生效")

		// 自动下载安装；state 已经是 downloading 说明是 CheckAndInstall 带起来的，别重复下载
		if s.app.Updater.State() == updater.StateAvailable && s.autoStarted.CompareAndSwap(false, true) {
			if isInstalledBuild() {
				// 安装版：下载 NSIS 安装包并校验，重启时由安装包接管升级
				//（不能用二进制替换——那会把安装版退化成便携版，还会留下 .old 残留）
				go s.downloadInstaller(rel)
			} else {
				go func() {
					if err := s.app.Updater.DownloadAndInstall(context.Background()); err != nil {
						log.Printf("自动下载更新失败: %v", err)
					}
				}()
			}
		}
	})

	s.app.Event.On(updater.EventDownloadProgress, func(e *application.CustomEvent) {
		p, _ := e.Data.(updater.Progress)
		s.mu.Lock()
		s.progress = p
		s.mu.Unlock()
		s.emit(updateProgressEvent, p)
	})

	s.app.Event.On(updater.EventUpdateReady, func(_ *application.CustomEvent) {
		// 安装前最后一道闸：重新计算已下载二进制的 SHA-256，
		// 与 release 按发行包名发布的校验文件中该资产的哈希比对；不一致则拒绝重启生效。
		// （updater 下载时已做过一次流式校验，这里按发布源独立复核。）
		if err := s.verifyDownloaded(); err != nil {
			s.verified.Store(false)
			s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "verify", Message: err.Error()})
			notifyUserTo(target(), "error", "更新校验失败", err.Error()+"，已阻止重启生效")
			log.Printf("更新二次校验失败: %v", err)
			return
		}
		s.verified.Store(true)
		s.emitStatus(updateReadyEvent)
		notifyUserTo(target(), "success", "更新已就绪", "重启 RcloneDrive 后生效")
	})

	s.app.Event.On(updater.EventNoUpdate, func(_ *application.CustomEvent) {
		s.emitStatus(updateLatestEvent)
	})

	s.app.Event.On(updater.EventError, func(e *application.CustomEvent) {
		info, _ := e.Data.(updater.ErrorInfo)
		s.emit(updateErrorEvent, UpdateErrorEvent{Stage: string(info.Stage), Message: info.Message})
		log.Printf("更新失败(stage=%s): %s", info.Stage, info.Message)
		// 下载失败后复位自动下载标记，下次检查（启动时或周期检查）会重试；
		// 成功（ready）时保持 true，避免已就绪后重复下载
		s.autoStarted.Store(false)
	})
}

// verifyDownloaded 复核已下载文件与 release 公布摘要的一致性。
func (s *UpdateService) verifyDownloaded() error {
	s.mu.Lock()
	release := s.release
	s.mu.Unlock()
	if release == nil {
		return errors.New("缺少新版本信息")
	}
	if release.Verification == nil || len(release.Verification.Digest) == 0 {
		return errors.New("release 未提供校验信息（" + release.Artifact.Filename + ".sha256），无法确认二进制来源")
	}
	path := s.app.Updater.DownloadedPath()
	if path == "" {
		return errors.New("未找到已下载的更新文件")
	}
	return verifyFileDigest(path, release.Verification.Digest)
}

// verifyFileDigest 计算文件的 SHA-256 并与 release 公布的摘要比对。
func verifyFileDigest(path string, digest []byte) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开已下载的更新文件失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("读取已下载的更新文件失败: %w", err)
	}
	if !bytes.Equal(h.Sum(nil), digest) {
		return errors.New("已下载文件与发布版本校验和不一致")
	}
	return nil
}

// downloadInstaller 安装版的下载流程：把 NSIS 安装包下载到临时目录，
// 校验 SHA-256 通过后经 update:ready 事件等待用户重启，重启时执行静默安装。
// Wails updater 的 DownloadAndInstall 只会做二进制替换，安装版必须走安装包。
func (s *UpdateService) downloadInstaller(rel *updater.Release) {
	urlStr, _ := rel.Metadata["github.asset.url"].(string)
	if urlStr == "" {
		s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "download", Message: "缺少安装包下载地址"})
		s.autoStarted.Store(false)
		return
	}
	dir, err := os.MkdirTemp("", "rclonedrive-update")
	if err != nil {
		s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "download", Message: err.Error()})
		s.autoStarted.Store(false)
		return
	}
	path := filepath.Join(dir, rel.Artifact.Filename)
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()

	s.mu.Lock()
	s.progress = updater.Progress{Total: rel.Artifact.Size}
	s.mu.Unlock()
	s.emit(updateProgressEvent, s.progress)

	if err = s.downloadFile(urlStr, path, rel.Artifact.Size); err != nil {
		s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "download", Message: err.Error()})
		s.autoStarted.Store(false)
		return
	}
	if rel.Verification == nil || len(rel.Verification.Digest) == 0 {
		err = errors.New("release 未提供校验信息（" + rel.Artifact.Filename + ".sha256），无法确认安装包来源")
		s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "verify", Message: err.Error()})
		s.autoStarted.Store(false)
		return
	}
	if err = verifyFileDigest(path, rel.Verification.Digest); err != nil {
		s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "verify", Message: err.Error()})
		s.autoStarted.Store(false)
		return
	}

	s.mu.Lock()
	s.stagedInstaller = path
	s.mu.Unlock()
	s.verified.Store(true)
	s.emitStatus(updateReadyEvent)
	notifyUserTo(s.currentToastTarget(), "success", "更新已就绪", "重启 RcloneDrive 后将自动完成安装")
}

// downloadFile 下载 url 到 path，期间按 ~500ms 节流推送进度事件。
func (s *UpdateService) downloadFile(urlStr, path string, total int64) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: updateHTTPTimeout, Transport: updateTransport()}).Do(req)
	if err != nil {
		return fmt.Errorf("下载安装包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载安装包失败: HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	var written int64
	var lastEmit time.Time
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if time.Since(lastEmit) > 500*time.Millisecond {
				s.reportProgress(written, total)
				lastEmit = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("下载安装包中断: %w", rerr)
		}
	}
	s.reportProgress(written, total)
	return nil
}

func (s *UpdateService) reportProgress(written, total int64) {
	s.mu.Lock()
	s.progress = updater.Progress{Written: written, Total: total}
	s.mu.Unlock()
	s.emit(updateProgressEvent, s.progress)
}

// Status 返回当前更新状态（设置窗口打开时初始化用）。
func (s *UpdateService) Status() UpdateStatus {
	s.mu.Lock()
	staged := s.stagedInstaller
	s.mu.Unlock()
	status := UpdateStatus{Current: appVersion, Verified: s.verified.Load(), Downloaded: staged != ""}
	if s.app == nil || s.app.Updater == nil {
		return status
	}
	status.State = string(s.app.Updater.State())
	if !status.Downloaded {
		status.Downloaded = s.app.Updater.DownloadedPath() != ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.release != nil {
		status.Latest = s.release.Version
		status.Notes = s.release.Notes
		if url, ok := s.release.Metadata["github.release.htmlURL"].(string); ok {
			status.URL = url
		}
	}
	return status
}

// CheckManual 触发一次更新检查（设置窗口的「检查更新」按钮）。
// 用户主动触发时给出明确反馈：无更新、出错都会通知；发现新版本则由
// 事件处理器通知并自动下载。Check 是同步的，返回时状态已确定。
// 提示定向到设置窗口（见 subscribe 的 toastTarget）。
func (s *UpdateService) CheckManual() {
	s.checkManual(settingsWindowName)
}

// checkFromTray 托盘菜单发起的检查，提示定向到主窗口。
func (s *UpdateService) checkFromTray() {
	s.checkManual("main")
}

func (s *UpdateService) checkManual(target string) {
	if !autoUpdateEnabled {
		log.Printf("开发构建：跳过手动检查更新")
		return
	}
	if s.app == nil || s.app.Updater == nil {
		return
	}
	s.toastTarget.Store(target)
	_, err := s.app.Updater.Check(context.Background())
	if err != nil {
		notifyUserTo(target, "error", "检查更新失败", err.Error())
		return
	}
	if s.app.Updater.State() == updater.StateUpToDate {
		notifyUserTo(target, "success", "检查更新", "已是最新版本 v"+appVersion)
	}
}

// Restart 重启应用使更新生效（仅在 update-ready 且二次校验通过后可用）。
// 安装版：退出后由安装包静默接管（installAfterExit），不替换二进制。
func (s *UpdateService) Restart() error {
	s.mu.Lock()
	staged := s.stagedInstaller
	s.mu.Unlock()
	if staged != "" {
		if !s.verified.Load() {
			return errors.New("安装包校验未通过，已阻止重启生效")
		}
		if err := installAfterExit(staged); err != nil {
			return err
		}
		s.app.Quit()
		return nil
	}
	if s.app == nil || s.app.Updater == nil {
		return nil
	}
	if s.app.Updater.DownloadedPath() != "" && !s.verified.Load() {
		return errors.New("更新文件校验未通过，已阻止重启生效")
	}
	return s.app.Updater.Restart(context.Background())
}

// emitStatus 以当前状态为负载发出更新事件。
func (s *UpdateService) emitStatus(name string) {
	s.emit(name, s.Status())
}

func (s *UpdateService) emit(name string, payload any) {
	if s.app != nil {
		s.app.Event.Emit(name, payload)
	}
}

// Progress 返回最近一次下载进度（Status 之外的补充，供前端轮询兜底）。
func (s *UpdateService) Progress() updater.Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

// portableAssetMatcher 选出用于自更新的 release 资产。
// updater 用下载到的字节直接替换正在运行的 exe，必须选便携版裸二进制，
// 绝不能选 NSIS 安装包（DefaultAssetMatcher 只排除 "installer" 字样，
// 会错误命中 CI 发布的 ...-setup.exe）。
func portableAssetMatcher(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	plat := strings.ToLower(req.Platform)
	arch := strings.ToLower(req.Arch)

	// 第一遍：便携版资产，要求同时命中平台与架构
	for i, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.Contains(name, "-portable") {
			continue
		}
		if plat != "" && !strings.Contains(name, plat) {
			continue
		}
		if arch != "" && !containsArch(name, arch) {
			continue
		}
		return i
	}
	// 兜底：没有便携版资产时，在排除安装包/校验附带文件后按默认规则匹配。
	// 选不到就返回 -1（该版本不提供自更新），也不能错选安装包——
	// updater 会用下载到的字节直接替换运行中的 exe，写成安装包就把应用换坏了。
	filtered := make([]github.ReleaseAsset, 0, len(assets))
	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if strings.Contains(name, "-setup.") || strings.Contains(name, "_setup.") ||
			strings.Contains(name, "-installer.") || strings.Contains(name, "_installer.") {
			continue
		}
		filtered = append(filtered, a)
	}
	idx := github.DefaultAssetMatcher(req, filtered)
	if idx < 0 {
		return -1
	}
	for i, a := range assets {
		if a.Name == filtered[idx].Name {
			return i
		}
	}
	return -1
}

// updateAssetMatcher 按当前分发形态选资产：安装版（NSIS）选 setup.exe
// 走安装流程，便携版选裸二进制走替换流程。
func updateAssetMatcher(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	if isInstalledBuild() {
		return setupAssetMatcher(req, assets)
	}
	return portableAssetMatcher(req, assets)
}

// setupAssetMatcher 选出 NSIS 安装包资产（要求同时命中平台与架构）。
// 安装版自更新靠安装包接管，选不到就返回 -1（该版本不提供安装包更新）。
func setupAssetMatcher(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	plat := strings.ToLower(req.Platform)
	arch := strings.ToLower(req.Arch)
	for i, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.Contains(name, "-setup.") && !strings.Contains(name, "_setup.") {
			continue
		}
		if plat != "" && !strings.Contains(name, plat) {
			continue
		}
		if arch != "" && !containsArch(name, arch) {
			continue
		}
		return i
	}
	return -1
}

// containsArch 判断资产名是否命中架构（含常见别名）。
func containsArch(name, arch string) bool {
	if strings.Contains(name, arch) {
		return true
	}
	switch arch {
	case "amd64":
		return strings.Contains(name, "x86_64") || strings.Contains(name, "x64")
	case "arm64":
		return strings.Contains(name, "aarch64")
	}
	return false
}
