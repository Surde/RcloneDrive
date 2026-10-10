package main

import (
	"net/url"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// 子窗口的名称与尺寸（无边框、固定尺寸，标题栏由前端自绘）。
const (
	settingsWindowName = "settings"
	settingsWindowW    = 520
	settingsWindowH    = 520
	driveWindowName    = "drive"
	driveWindowW       = 760
	driveWindowH       = 680
)

// driveOpenEvent 通知已经打开的驱动器窗口切换到新的目标（新建或编辑某个驱动器）。
// 新建一个 WebView2 窗口要 1 秒以上（创建环境 + 控制器 + 加载页面），
// 所以子窗口只在首次打开时创建，之后靠事件切换内容。
const driveOpenEvent = "drive:open"

// DriveOpenEvent 是 drive:open 事件的负载；Name 为空表示新建驱动器。
type DriveOpenEvent struct {
	Name string `json:"name"`
}

func init() {
	application.RegisterEvent[DriveOpenEvent](driveOpenEvent)
}

// WindowService 负责打开设置、新增/编辑驱动器等独立窗口；已打开时复用并聚焦。
type WindowService struct {
	mu      sync.Mutex
	windows map[string]*application.WebviewWindow
	// main 是主窗口，用于子窗口关闭后把主界面带到前台
	main *application.WebviewWindow
	// driveAddMode 记录驱动器窗口当前是否为「新增」模式（关闭后要显示主界面）
	driveAddMode bool
}

// NewWindowService 创建窗口服务。
func NewWindowService() *WindowService {
	return &WindowService{windows: make(map[string]*application.WebviewWindow)}
}

// setMainWindow 记录主窗口；主窗口可能被隐藏到托盘，需要由子窗口收尾时唤起。
func (s *WindowService) setMainWindow(window *application.WebviewWindow) {
	s.mu.Lock()
	s.main = window
	s.mu.Unlock()
}

// showMainWindow 显示并聚焦主窗口。
func (s *WindowService) showMainWindow() {
	s.mu.Lock()
	main := s.main
	s.mu.Unlock()
	if main == nil {
		return
	}
	main.Show()
	main.Focus()
}

// OpenSettings 打开（或聚焦）设置窗口。
func (s *WindowService) OpenSettings() {
	if s.reuse(settingsWindowName, "", nil) {
		return
	}
	s.create(settingsWindowName, "/#/settings", settingsWindowW, settingsWindowH)
}

// OpenAddDrive 打开（或聚焦）新增驱动器窗口。
func (s *WindowService) OpenAddDrive() {
	s.openDriveWindow("")
}

// OpenEditDrive 打开（或聚焦）编辑驱动器窗口，name 为现有远程名称。
// 驱动器已连接时的确认与断开由界面负责（见 DriveCard.vue）。
func (s *WindowService) OpenEditDrive(name string) {
	s.openDriveWindow(name)
}

// openDriveWindow 打开驱动器窗口；窗口已创建时只投递事件切换目标，不重新加载页面。
func (s *WindowService) openDriveWindow(name string) {
	addMode := name == ""
	s.mu.Lock()
	s.driveAddMode = addMode
	s.mu.Unlock()

	if s.reuse(driveWindowName, driveOpenEvent, DriveOpenEvent{Name: name}) {
		return
	}
	path := "/#/drive"
	if name != "" {
		path = "/#/drive?edit=" + url.QueryEscape(name)
	}
	s.create(driveWindowName, path, driveWindowW, driveWindowH)
}

// reuse 复用已创建的窗口：必要时先投递事件让界面切换到目标内容，然后显示并聚焦。
func (s *WindowService) reuse(name, event string, payload any) bool {
	s.mu.Lock()
	window := s.windows[name]
	s.mu.Unlock()
	if window == nil {
		return false
	}
	if event != "" {
		application.Get().Event.Emit(event, payload)
	}
	window.Show()
	window.Focus()
	return true
}

func (s *WindowService) create(name, path string, width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	window := application.Get().Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             name,
		Title:            "RcloneDrive",
		Width:            width,
		Height:           height,
		DisableResize:    true,
		Frameless:        true,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              path,
	})
	// 关闭只隐藏：窗口与其 WebView2 保持存活，下次打开直接复用（见 reuse）。
	// 新增驱动器窗口关闭后（取消或保存）把主界面带到前台。
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
		if name != driveWindowName {
			return
		}
		s.mu.Lock()
		addMode := s.driveAddMode
		s.mu.Unlock()
		if addMode {
			s.showMainWindow()
		}
	})
	s.windows[name] = window
	window.Center()
}
