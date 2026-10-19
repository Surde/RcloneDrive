package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"golang.org/x/mod/semver"
)

// 更新流程状态（对齐旧版 Wails updater 的状态机语义，Status().State 的值）：
// idle → checking → up-to-date（已是最新）或 available（发现新版本）→
// downloading → verifying → installing → ready（可重启生效）/ error。
const (
	stateIdle        = "idle"
	stateChecking    = "checking"
	stateUpToDate    = "up-to-date"
	stateAvailable   = "available"
	stateDownloading = "downloading"
	stateVerifying   = "verifying"
	stateInstalling  = "installing"
	stateReady       = "ready"
	stateError       = "error"
)

// updateCheckInterval 是后台自动检查更新的间隔。
const updateCheckInterval = 6 * time.Hour

// updateRepository 是发布 Release 的 GitHub 仓库。
const updateRepository = "younland/RcloneDrive"

// updateAPIBase 是 GitHub API 的地址（仅 HTTPS；Release 资产下载地址由 API 返回）。
const updateAPIBase = "https://api.github.com"

// updateHTTPTimeout 是更新相关 HTTP 请求的总超时。便携版二进制近 80MB，
// 默认 30s 总超时会掐断慢速网络下的下载；完整性靠 release 附带的 checksums 校验兜底。
const updateHTTPTimeout = 30 * time.Minute

// 更新通知用的系统通知标题（toast 由 Stage 3 的视图接管，见 notifyUserTo）。
// Stage 3 起「发现新版本 / 更新已就绪」等提示改回应用内 toast（旧版 app:toast 事件），
// 这里统一走 notifyUserTo → 系统通知，代码中保留 target 语义不变。

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

// UpdateStatus 描述当前更新状态，是 Status() 的返回（Stage 3 的 UI 直接消费）。
type UpdateStatus struct {
	Current    string `json:"current"`
	State      string `json:"state"` // idle/checking/up-to-date/available/downloading/verifying/installing/ready/error
	Latest     string `json:"latest,omitempty"`
	Notes      string `json:"notes,omitempty"`
	URL        string `json:"url,omitempty"`
	Downloaded bool   `json:"downloaded"`
	// Verified 为 true 表示已下载的更新与 release 公布的 SHA-256 校验一致，
	// 此时才允许重启生效；false 时「立即重启」不可用。
	Verified bool `json:"verified"`
}

// UpdateErrorEvent 描述一次更新失败（Stage 3 的错误视图消费）。
type UpdateErrorEvent struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// Progress 是下载进度（Status 之外的补充，供 UI 轮询兜底）。
type Progress struct {
	Written int64 `json:"written"`
	Total   int64 `json:"total"`
}

// matchedRelease 是检查阶段匹配到的一次可更新版本。
type matchedRelease struct {
	Version string // 去掉 v 前缀的 tag
	Notes   string
	URL     string // release 页面地址
	Asset   githubAsset
	Digest  []byte // 发行包对应的 SHA-256（取不到时为空，此时禁止下载）
}

// githubRelease / githubAsset 是 GitHub API 响应的解析目标。
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Body    string        `json:"body"`
	HTMLURL string        `json:"html_url"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"` // 302 到 CDN
	Size int64  `json:"size"`
}

// UpdateService 是纯 Go 的 GitHub Releases 更新器（替代旧版基于 Wails updater
// 的实现）：检查更新、下载并校验资产、按分发形态安装（便携版就地替换二进制 /
// 安装版下载 NSIS 安装包重启后静默安装）、重启生效。
// 状态变化通过 OnChange 回调多播（与 RemoteService/MountService 一致），
// UI 经 Status()/Progress() 主动取数。
type UpdateService struct {
	settings *SettingsService

	// httpClient 带系统代理与总超时，所有更新请求共用。
	httpClient *http.Client

	mu       sync.Mutex
	state    string
	release  *matchedRelease // 最近一次发现的新版本
	progress Progress        // 最近一次下载进度
	// stagedInstaller 是安装版已下载并校验通过的 NSIS 安装包路径；
	// 非空时 Restart 走「退出 → 静默安装 → 重启应用」而不是二进制替换。
	stagedInstaller string
	// downloadedPath 是便携版已下载（并可能解压）的就绪文件路径。
	downloadedPath string

	// autoStarted 防止重复自动下载（周期检查与手动检查可能同时命中可用版本）。
	autoStarted atomic.Bool
	// verified 记录最近一次下载完成后的二次校验结果，Restart 据此放行。
	verified atomic.Bool
	// toastTarget 记录最近一次检查的发起窗口，后续通知按它定向
	// （设置窗口发起的检查，提示出现在设置窗口的语义由 Stage 3 的视图接管）。
	toastTarget atomic.Value

	// observers 是状态变化回调（多播，mutex 保护）。
	observers []func()

	// stopCh 关闭时停止后台周期检查。
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewUpdateService 创建更新服务。
func NewUpdateService(settings *SettingsService) *UpdateService {
	return &UpdateService{
		settings:   settings,
		httpClient: &http.Client{Timeout: updateHTTPTimeout, Transport: updateTransport()},
		state:      stateIdle,
		stopCh:     make(chan struct{}),
	}
}

// OnChange 注册更新状态变化回调（用于 Stage 3 的界面重绘）；可注册多个，互不覆盖。
func (s *UpdateService) OnChange(fn func()) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, fn)
}

func (s *UpdateService) notifyChange() {
	s.mu.Lock()
	observers := make([]func(), len(s.observers))
	copy(observers, s.observers)
	s.mu.Unlock()
	for _, fn := range observers {
		fn()
	}
}

// setState 更新状态并通知观察者。
func (s *UpdateService) setState(state string) {
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	s.notifyChange()
}

// Start 启动更新服务：清理上次二进制替换升级遗留的 <exe>.old.* 文件，
// 并启动后台周期检查（AutoUpdate 设置开启时才真正做网络检查）。
// 开发构建不启动（见 autoupdate_dev.go），不做任何自动/手动更新。
func (s *UpdateService) Start() {
	if !autoUpdateEnabled {
		log.Printf("开发构建：自动更新已禁用")
		return
	}
	s.sweepStaleUpdateArtifacts()
	go s.autoCheckLoop()
}

// Stop 停止后台周期检查（应用退出时由 main 调用）。
func (s *UpdateService) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// sweepStaleUpdateArtifacts 清理上一次二进制替换升级遗留的 <exe>.old.* 文件。
// 就地替换时运行中的旧映像删不掉（Windows 内核持有），只能改名搁置、
// 下次启动时删除；应用重启后旧映像必然已释放，这里兜底删除。
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
	select {
	case <-time.After(10 * time.Second):
	case <-s.stopCh:
		return
	}
	if s.settings.AutoUpdate() {
		s.checkBackground()
	}
	ticker := time.NewTicker(updateCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if s.settings.AutoUpdate() {
				s.checkBackground()
			}
		case <-s.stopCh:
			return
		}
	}
}

// checkBackground 静默检查：有更新才通知，已是最新不打扰。
func (s *UpdateService) checkBackground() {
	s.toastTarget.Store("main")
	if err := s.check(context.Background()); err != nil {
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

// fetchLatestRelease 请求 GitHub 最新正式 Release（不含预发布与草稿）。
// 没有 Release 或全部被排除时返回 nil。
func (s *UpdateService) fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	endpoint := updateAPIBase + "/repos/" + updateRepository + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "RcloneDrive/"+currentAppVersion())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("检查更新失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// 仓库还没有任何 Release
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("检查更新失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("解析更新信息失败: %w", err)
	}
	if rel.TagName == "" {
		return nil, nil
	}
	return &rel, nil
}

// check 执行一次完整的检查：发现新版本后取回校验摘要（fail closed：
// 取不到就不允许下载），通知用户并自动下载安装。
func (s *UpdateService) check(ctx context.Context) error {
	s.setState(stateChecking)
	rel, err := s.fetchLatestRelease(ctx)
	if err != nil {
		s.fail("check", err)
		return err
	}
	if rel == nil || !isNewerVersion(rel.TagName, currentAppVersion()) {
		s.setState(stateUpToDate)
		return nil
	}

	// 按分发形态匹配资产：安装版（NSIS）选 setup.exe 走安装流程，
	// 便携版选裸二进制走就地替换流程
	idx := updateAssetMatcher(checkRequest{Platform: runtime.GOOS, Arch: runtime.GOARCH}, rel.Assets)
	if idx < 0 || idx >= len(rel.Assets) {
		err := fmt.Errorf("release %s 没有适配 %s/%s 的安装包", rel.TagName, runtime.GOOS, runtime.GOARCH)
		s.fail("check", err)
		return err
	}

	matched := &matchedRelease{
		Version: trimVPrefix(rel.TagName),
		Notes:   rel.Body,
		URL:     rel.HTMLURL,
		Asset:   rel.Assets[idx],
	}

	// 先取回本发行包的校验摘要：取不到就不允许下载，避免装上无法验证来源的更新
	digest, err := fetchSidecarDigest(ctx, rel.Assets[idx].URL+".sha256", rel.Assets[idx].Name, s.httpClient)
	if err != nil {
		s.fail("verify", err)
		notifyUserTo(s.currentToastTarget(), "error", "获取更新校验信息失败", err.Error())
		return err
	}
	matched.Digest = digest

	s.mu.Lock()
	s.release = matched
	s.progress = Progress{Total: matched.Asset.Size}
	s.mu.Unlock()
	s.verified.Store(false) // 新版本待下载，校验结果作废

	s.setState(stateAvailable)
	notifyUserTo(s.currentToastTarget(), "success", "发现新版本 v"+matched.Version, "开始自动下载，完成后重启生效")

	// 自动下载安装；autoStarted 防止周期检查与手动检查同时命中可用版本时重复下载
	if s.autoStarted.CompareAndSwap(false, true) {
		if isInstalledBuild() {
			// 安装版：下载 NSIS 安装包并校验，重启时由安装包接管升级
			//（不能用二进制替换——那会把安装版退化成便携版，还会留下 .old 残留）
			go s.downloadInstaller(ctx, matched)
		} else {
			go func() {
				if err := s.downloadAndStage(ctx, matched); err != nil {
					log.Printf("自动下载更新失败: %v", err)
				}
			}()
		}
	}
	return nil
}

// fail 进入 error 状态并通知观察者。
func (s *UpdateService) fail(stage string, err error) {
	log.Printf("更新失败(stage=%s): %v", stage, err)
	s.setState(stateError)
	// 下载失败后复位自动下载标记，下次检查（启动时或周期检查）会重试；
	// 成功（ready）时保持 true，避免已就绪后重复下载
	s.autoStarted.Store(false)
}

// isNewerVersion 按 SemVer 2.0.0 比较 tag 是否比当前版本新（容忍 v/V 前缀）。
// 非法版本串按 x/mod/semver 的稳定语义处理：互相相等、小于任何合法版本。
func isNewerVersion(tag, current string) bool {
	if trimVPrefix(tag) == "" {
		return false
	}
	if trimVPrefix(current) == "" {
		return true
	}
	return semver.Compare(canonicalVersion(tag), canonicalVersion(current)) > 0
}

// trimVPrefix 去掉 tag 开头的 v/V（GitHub 发布 tag 的惯例写法）。
func trimVPrefix(tag string) string {
	if strings.HasPrefix(tag, "v") || strings.HasPrefix(tag, "V") {
		return tag[1:]
	}
	return tag
}

// canonicalVersion 转成 x/mod/semver 期望的形式：单个前导 v。
func canonicalVersion(s string) string {
	s = trimVPrefix(s)
	if s == "" {
		return ""
	}
	return "v" + s
}

// checkRequest 描述运行平台（对应旧版 updater.CheckRequest 用到的字段）。
type checkRequest struct {
	Platform string
	Arch     string
}

// updateAssetMatcher 按当前分发形态选资产：安装版（NSIS）选 setup.exe
// 走安装流程，便携版选裸二进制走替换流程。
func updateAssetMatcher(req checkRequest, assets []githubAsset) int {
	if isInstalledBuild() {
		return setupAssetMatcher(req, assets)
	}
	return portableAssetMatcher(req, assets)
}

// portableAssetMatcher 选出用于自更新的 release 资产。
// 更新器用下载到的字节直接替换正在运行的 exe，必须选便携版裸二进制，
// 绝不能选 NSIS 安装包（默认匹配只排除 "installer" 字样，
// 会错误命中 CI 发布的 ...-setup.exe）。
func portableAssetMatcher(req checkRequest, assets []githubAsset) int {
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
	// 兜底：没有便携版资产时，在排除安装包/校验附带文件后按「平台+架构」匹配。
	// 选不到就返回 -1（该版本不提供自更新），也不能错选安装包——
	// 更新器会用下载到的字节直接替换运行中的 exe，写成安装包就把应用换坏了。
	for i, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.Contains(name, "-setup.") && !strings.Contains(name, "_setup.") &&
			!strings.Contains(name, "-installer.") && !strings.Contains(name, "_installer.") {
			continue
		}
		if strings.HasSuffix(name, ".sha256") {
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

// setupAssetMatcher 选出 NSIS 安装包资产（要求同时命中平台与架构）。
// 安装版自更新靠安装包接管，选不到就返回 -1（该版本不提供安装包更新）。
func setupAssetMatcher(req checkRequest, assets []githubAsset) int {
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

// fetchSidecarDigest 下载 <发行包名>.sha256 并解析出摘要。
// 文件内容为 sha256sum 标准格式「<哈希>  <文件名>」，也兼容只有哈希的单行形式；
// 带文件名时核对必须与目标发行包一致，防止张冠李戴。
func fetchSidecarDigest(ctx context.Context, urlStr, wantName string, client *http.Client) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
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

// downloadAndStage 便携版的下载流程：把资产下载到临时目录、流式校验 SHA-256，
// 压缩包（.zip/.tar.gz）就地解压出单个顶层条目，就绪后等待用户重启生效。
func (s *UpdateService) downloadAndStage(ctx context.Context, rel *matchedRelease) error {
	s.setState(stateDownloading)
	s.reportProgress(0, rel.Asset.Size)

	dir, err := os.MkdirTemp("", "rclonedrive-update")
	if err != nil {
		s.fail("download", err)
		return err
	}
	// 成功后由 Restart 消费里面的文件；失败时清理整个目录
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()

	path := filepath.Join(dir, rel.Asset.Name)
	if err := s.downloadFile(ctx, rel.Asset.URL, path, rel.Asset.Size); err != nil {
		s.fail("download", err)
		return err
	}

	s.setState(stateVerifying)
	if err := verifyFileDigest(path, rel.Digest); err != nil {
		s.fail("verify", err)
		return err
	}

	s.setState(stateInstalling)
	finalPath, err := maybeExtractInto(path)
	if err != nil {
		s.fail("install", err)
		return err
	}

	// 安装前最后一道闸：重新计算已下载文件的 SHA-256 与发布摘要复核
	// （下载时做过一次流式校验，这里按发布源独立复核）。
	if err := verifyFileDigest(finalPath, rel.Digest); err != nil {
		s.verified.Store(false)
		s.fail("verify", err)
		notifyUserTo(s.currentToastTarget(), "error", "更新校验失败", err.Error()+"，已阻止重启生效")
		return err
	}

	s.mu.Lock()
	s.downloadedPath = finalPath
	s.mu.Unlock()
	s.verified.Store(true)
	ok = true
	s.setState(stateReady)
	notifyUserTo(s.currentToastTarget(), "success", "更新已就绪", "重启 RcloneDrive 后生效")
	return nil
}

// downloadInstaller 安装版的下载流程：把 NSIS 安装包下载到临时目录，
// 校验 SHA-256 通过后经 Restart 等待用户重启，重启时执行静默安装。
func (s *UpdateService) downloadInstaller(ctx context.Context, rel *matchedRelease) {
	s.setState(stateDownloading)
	s.reportProgress(0, rel.Asset.Size)

	dir, err := os.MkdirTemp("", "rclonedrive-update")
	if err != nil {
		s.fail("download", err)
		return
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()

	path := filepath.Join(dir, rel.Asset.Name)
	if err := s.downloadFile(ctx, rel.Asset.URL, path, rel.Asset.Size); err != nil {
		s.fail("download", err)
		return
	}

	s.setState(stateVerifying)
	if err := verifyFileDigest(path, rel.Digest); err != nil {
		s.fail("verify", err)
		return
	}

	s.mu.Lock()
	s.stagedInstaller = path
	s.mu.Unlock()
	s.verified.Store(true)
	ok = true
	s.setState(stateReady)
	notifyUserTo(s.currentToastTarget(), "success", "更新已就绪", "重启 RcloneDrive 后将自动完成安装")
}

// downloadFile 下载 url 到 path，期间按 ~500ms 节流更新进度。
func (s *UpdateService) downloadFile(ctx context.Context, urlStr, path string, total int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "RcloneDrive/"+currentAppVersion())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载更新失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载更新失败: HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	var written int64
	var lastReport time.Time
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if time.Since(lastReport) > 500*time.Millisecond {
				s.reportProgress(written, total)
				lastReport = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("下载更新中断: %w", rerr)
		}
	}
	s.reportProgress(written, total)
	return nil
}

func (s *UpdateService) reportProgress(written, total int64) {
	s.mu.Lock()
	s.progress = Progress{Written: written, Total: total}
	s.mu.Unlock()
	// 进度不频繁通知（500ms 节流在 downloadFile 里），UI 经 Progress() 轮询兜底
	s.notifyChange()
}

// verifyFileDigest 计算文件的 SHA-256 并与 release 公布的摘要比对。
func verifyFileDigest(path string, digest []byte) error {
	if len(digest) == 0 {
		return errors.New("release 未提供校验信息（.sha256），无法确认更新来源")
	}
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

// archiveKind 是更新资产支持的压缩包格式。
type archiveKind int

const (
	archiveNone archiveKind = iota
	archiveZip
	archiveTarGz
)

// 解压限额：单个 macOS .app 包通常 <100MiB、几千个条目，
// 封顶防止畸形/恶意压缩包耗尽磁盘或文件描述符。
const (
	maxArchiveEntries         = 50_000
	maxArchiveTotalSize int64 = 2 << 30 // 2 GiB 总解压大小
)

// detectArchive 按扩展名识别压缩包（CDN 都会保留扩展名；
// 保守识别：只有扩展名明确时才解压，误判会静默搞坏替换流程）。
func detectArchive(path string) archiveKind {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return archiveZip
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return archiveTarGz
	}
	return archiveNone
}

// maybeExtractInto 就地解包 path 处的压缩包：解压到同级临时目录，
// 删掉原压缩包，把唯一的顶层条目移到原位置。非压缩包原样返回。
// 约束与旧版 Wails updater 一致：只允许一个顶层条目（一个 .app 包或一个二进制）、
// 拒绝路径穿越（zip-slip）、拒绝指向解压目录外的符号链接、限额封顶。
func maybeExtractInto(stagedPath string) (string, error) {
	kind := detectArchive(stagedPath)
	if kind == archiveNone {
		return stagedPath, nil
	}

	parent := filepath.Dir(stagedPath)
	scratch, err := os.MkdirTemp(parent, ".payload-*")
	if err != nil {
		return "", fmt.Errorf("解压更新包失败（创建临时目录）: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(scratch) }

	var entries []string
	switch kind {
	case archiveZip:
		entries, err = extractZip(stagedPath, scratch)
	case archiveTarGz:
		entries, err = extractTarGz(stagedPath, scratch)
	}
	if err != nil {
		cleanup()
		return "", fmt.Errorf("解压更新包失败: %w", err)
	}
	if len(entries) != 1 {
		cleanup()
		return "", fmt.Errorf("更新包必须只含一个顶层条目，实际 %d 个", len(entries))
	}
	if err := os.Remove(stagedPath); err != nil {
		cleanup()
		return "", err
	}
	top := filepath.Join(scratch, entries[0])
	dst := filepath.Join(parent, entries[0])
	if err := os.Rename(top, dst); err != nil {
		cleanup()
		return "", err
	}
	cleanup()
	return dst, nil
}

// extractZip 把 zip 解到 dest，返回顶层条目名（去重）。
func extractZip(src, dest string) ([]string, error) {
	r, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if len(r.File) > maxArchiveEntries {
		return nil, errors.New("更新包条目数超限")
	}
	var total int64
	top := map[string]bool{}
	for _, f := range r.File {
		name, err := safeRelPath(f.Name)
		if err != nil {
			return nil, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(filepath.Join(dest, filepath.Dir(name)), 0o755); err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		total, err = writeCapped(filepath.Join(dest, name), rc, f.FileInfo().Mode(), total)
		rc.Close()
		if err != nil {
			return nil, err
		}
		top[topName(name)] = true
	}
	return keysOf(top), nil
}

// extractTarGz 把 .tar.gz 解到 dest，返回顶层条目名（去重）。
func extractTarGz(src, dest string) ([]string, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	count := 0
	top := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		count++
		if count > maxArchiveEntries {
			return nil, errors.New("更新包条目数超限")
		}
		name, err := safeRelPath(hdr.Name)
		if err != nil {
			return nil, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Join(dest, filepath.Dir(name)), 0o755); err != nil {
				return nil, err
			}
			total, err = writeCapped(filepath.Join(dest, name), tr, os.FileMode(hdr.Mode&0o777), total)
			if err != nil {
				return nil, err
			}
		case tar.TypeSymlink, tar.TypeLink:
			// 链接目标按相对 name 的目录解析，规格化后不允许逃出解压目录（zip-slip 的链接变体）
			linkTarget := filepath.Clean(filepath.Join(filepath.Dir(name), hdr.Linkname))
			if filepath.IsAbs(linkTarget) || linkTarget == ".." || strings.HasPrefix(linkTarget, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("更新包内链接越界: %s -> %s", name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Join(dest, filepath.Dir(name)), 0o755); err != nil {
				return nil, err
			}
			if hdr.Typeflag == tar.TypeSymlink {
				err = os.Symlink(hdr.Linkname, filepath.Join(dest, name))
			} else {
				err = os.Link(filepath.Join(dest, linkTarget), filepath.Join(dest, name))
			}
			if err != nil {
				return nil, err
			}
		default:
			continue
		}
		top[topName(name)] = true
	}
	return keysOf(top), nil
}

// safeRelPath 校验压缩包内路径：规格化后不允许逃出解压目录。
func safeRelPath(name string) (string, error) {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("更新包内路径越界: %s", name)
	}
	return clean, nil
}

// topName 取规格化路径的第一段作为顶层条目名。
func topName(name string) string {
	top, _, _ := strings.Cut(name, string(filepath.Separator))
	return top
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// writeCapped 把 r 写到 path（带执行位），累计解压大小封顶。
func writeCapped(path string, r io.Reader, mode os.FileMode, total int64) (int64, error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return total, err
	}
	defer out.Close()
	n, err := io.Copy(out, io.LimitReader(r, maxArchiveTotalSize-total+1))
	total += n
	if total > maxArchiveTotalSize {
		return total, errors.New("更新包解压后总大小超限")
	}
	return total, err
}

// Status 返回当前更新状态（设置窗口打开时初始化用）。
func (s *UpdateService) Status() UpdateStatus {
	s.mu.Lock()
	release := s.release
	staged := s.stagedInstaller
	downloaded := s.downloadedPath
	state := s.state
	s.mu.Unlock()
	status := UpdateStatus{
		Current:    currentAppVersion(),
		State:      state,
		Downloaded: staged != "" || downloaded != "",
		Verified:   s.verified.Load(),
	}
	if release != nil {
		status.Latest = release.Version
		status.Notes = release.Notes
		status.URL = release.URL
	}
	return status
}

// Progress 返回最近一次下载进度（Status 之外的补充，供 UI 轮询兜底）。
func (s *UpdateService) Progress() Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

// CheckManual 触发一次更新检查（设置窗口的「检查更新」按钮）。
// 用户主动触发时给出明确反馈：无更新、出错都会通知；发现新版本则由
// check 内部的流程通知并自动下载。
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
	if target == "" {
		target = "main"
	}
	s.toastTarget.Store(target)
	err := s.check(context.Background())
	if err != nil {
		notifyUserTo(target, "error", "检查更新失败", err.Error())
		return
	}
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	if state == stateUpToDate {
		notifyUserTo(target, "success", "检查更新", "已是最新版本 v"+currentAppVersion())
	}
}

// Restart 重启应用使更新生效（仅在 ready 且二次校验通过后可用）。
// 安装版：退出后由安装包静默接管（installAfterExit），不替换二进制；
// 便携版：就地替换二进制/应用包后由 mygo 重启应用（App.Relaunch）。
func (s *UpdateService) Restart() error {
	s.mu.Lock()
	staged := s.stagedInstaller
	downloaded := s.downloadedPath
	s.mu.Unlock()

	if staged != "" {
		if !s.verified.Load() {
			return errors.New("安装包校验未通过，已阻止重启生效")
		}
		if err := installAfterExit(staged); err != nil {
			return err
		}
		mygo.App.Quit()
		return nil
	}
	if downloaded == "" {
		return errors.New("没有已下载的更新")
	}
	if !s.verified.Load() {
		return errors.New("更新文件校验未通过，已阻止重启生效")
	}
	if err := applyUpdate(downloaded); err != nil {
		return err
	}
	mygo.App.Relaunch()
	return nil
}

// applyUpdate 便携版就地替换：把已下载（解压）的文件换到当前可执行文件的位置。
// Windows 下运行中的 exe 删不掉但可以改名（内核仍映射旧映像），
// 所以先改名搁置（<exe>.old.<时间戳>，下次启动清扫），再把新文件换到位；
// Unix 直接删除重命名（打开的文件句柄仍指向旧 inode）。
func applyUpdate(newPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位当前程序失败: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("定位当前程序失败: %w", err)
	}
	target := bundleTarget(self)

	// 记录原文件的权限位：下载文件是 os.Create 建的（0o666 减 umask），
	// 直接换名会丢执行位（Unix 上 exec 会 EACCES）
	origMode := os.FileMode(0)
	if info, err := os.Stat(target); err == nil {
		origMode = info.Mode()
	}

	if runtime.GOOS == "windows" {
		if err := replaceRunningBinary(target, newPath); err != nil {
			return err
		}
	} else {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("替换程序文件失败: %w", err)
		}
		if err := os.Rename(newPath, target); err != nil {
			return fmt.Errorf("替换程序文件失败: %w", err)
		}
	}
	if origMode != 0 {
		if err := os.Chmod(target, origMode.Perm()); err != nil {
			log.Printf("恢复程序文件权限失败（非致命）: %v", err)
		}
	}
	return nil
}

// replaceRunningBinary 是 Windows 的就地替换：删除失败时（映像仍被内核持有）
// 把运行中的文件改名搁置，让出新文件的位置；最多重试 20 次（每次 500ms），
// 仍失败则把原文件改回去，防止半替换状态。
func replaceRunningBinary(target, newPath string) error {
	// 第一次尝试：没有进程占用时直接删除最干净（不留 .old 残留）
	if err := os.RemoveAll(target); err == nil {
		sweepRenameAsides(target)
		return os.Rename(newPath, target)
	}

	for i := 0; i < 20; i++ {
		aside := fmt.Sprintf("%s.old.%d", target, time.Now().UnixNano())
		if err := os.Rename(target, aside); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err := os.Rename(newPath, target); err != nil {
			// 换回去，防止半替换状态
			_ = os.Rename(aside, target)
			return fmt.Errorf("替换程序文件失败: %w", err)
		}
		_ = os.Remove(aside) // 通常失败（映像未释放），由下次启动清扫
		sweepRenameAsides(target)
		return nil
	}
	return errors.New("替换程序文件失败：文件被占用，请稍后重试")
}

// sweepRenameAsides 尽力删除 <target>.old.* 残留（上次升级遗留的旧映像）。
func sweepRenameAsides(target string) {
	matches, err := filepath.Glob(target + ".old.*")
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// bundleTarget 返回要替换的目标：macOS 上运行在 .app 包里时替换整个 .app
// 目录（Release 资产里也是 .app 包），否则替换可执行文件本身。
func bundleTarget(executable string) string {
	if runtime.GOOS == "darwin" {
		const marker = ".app/Contents/MacOS/"
		if i := strings.Index(executable, marker); i > 0 {
			return executable[:i+len(".app")]
		}
	}
	return executable
}
