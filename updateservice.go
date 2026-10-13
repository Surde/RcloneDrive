package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

	// autoStarted 防止重复自动下载（周期检查与手动检查可能同时命中可用事件）。
	autoStarted atomic.Bool
	// verified 记录最近一次下载完成后的二次校验结果，Restart 据此放行。
	verified atomic.Bool
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
		AssetMatcher: portableAssetMatcher,
		// release 附带 checksums.txt：下载被中断（连接被干净地提前 EOF）时
		// 提供商会当作成功返回，校验和是兜住部分文件的最后防线
		ChecksumAsset: "checksums.txt",
		HTTPClient:    &http.Client{Timeout: updateHTTPTimeout},
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
	go s.autoCheckLoop()
	return nil
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
	if _, err := s.app.Updater.Check(context.Background()); err != nil {
		log.Printf("检查更新失败: %v", err)
	}
}

// subscribe 把 updater 的生命周期事件翻译成 project 内事件与通知。
func (s *UpdateService) subscribe() {
	s.app.Event.On(updater.EventUpdateAvailable, func(e *application.CustomEvent) {
		rel, _ := e.Data.(*updater.Release)
		if rel == nil {
			return
		}
		s.mu.Lock()
		s.release = rel
		s.progress = updater.Progress{Total: rel.Artifact.Size}
		s.mu.Unlock()
		s.verified.Store(false) // 新版本待下载，校验结果作废

		s.emitStatus(updateAvailableEvent)
		notifyUser("success", "发现新版本 v"+rel.Version, "开始自动下载，完成后重启生效")

		// 自动下载安装；state 已经是 downloading 说明是 CheckAndInstall 带起来的，别重复下载
		if s.app.Updater.State() == updater.StateAvailable && s.autoStarted.CompareAndSwap(false, true) {
			go func() {
				if err := s.app.Updater.DownloadAndInstall(context.Background()); err != nil {
					log.Printf("自动下载更新失败: %v", err)
				}
			}()
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
		// 与 release 附带 checksums.txt 中该资产的哈希比对；不一致则拒绝重启生效。
		// （updater 下载时已做过一次流式校验，这里按发布源独立复核。）
		if err := s.verifyDownloaded(); err != nil {
			s.verified.Store(false)
			s.emit(updateErrorEvent, UpdateErrorEvent{Stage: "verify", Message: err.Error()})
			notifyUser("error", "更新校验失败", err.Error()+"，已阻止重启生效")
			log.Printf("更新二次校验失败: %v", err)
			return
		}
		s.verified.Store(true)
		s.emitStatus(updateReadyEvent)
		notifyUser("success", "更新已就绪", "重启 RcloneDrive 后生效")
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
		return errors.New("release 未提供校验信息（checksums.txt），无法确认二进制来源")
	}
	path := s.app.Updater.DownloadedPath()
	if path == "" {
		return errors.New("未找到已下载的更新文件")
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
	if !bytes.Equal(h.Sum(nil), release.Verification.Digest) {
		return errors.New("已下载文件与发布版本校验和不一致")
	}
	return nil
}

// Status 返回当前更新状态（设置窗口打开时初始化用）。
func (s *UpdateService) Status() UpdateStatus {
	status := UpdateStatus{Current: appVersion, Verified: s.verified.Load()}
	if s.app == nil || s.app.Updater == nil {
		return status
	}
	status.State = string(s.app.Updater.State())
	status.Downloaded = s.app.Updater.DownloadedPath() != ""

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

// CheckManual 触发一次更新检查（托盘菜单 / 设置按钮）。
// 用户主动触发时给出明确反馈：无更新、出错都会通知；发现新版本则由
// 事件处理器通知并自动下载。Check 是同步的，返回时状态已确定。
func (s *UpdateService) CheckManual() {
	if !autoUpdateEnabled {
		log.Printf("开发构建：跳过手动检查更新")
		return
	}
	if s.app == nil || s.app.Updater == nil {
		return
	}
	_, err := s.app.Updater.Check(context.Background())
	if err != nil {
		notifyUser("error", "检查更新失败", err.Error())
		return
	}
	if s.app.Updater.State() == updater.StateUpToDate {
		notifyUser("success", "检查更新", "已是最新版本 v"+appVersion)
	}
}

// Restart 重启应用使更新生效（仅在 update-ready 且二次校验通过后可用）。
func (s *UpdateService) Restart() error {
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
