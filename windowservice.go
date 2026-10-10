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

// WindowService 负责打开设置、新增/编辑驱动器等独立窗口；已打开时复用并聚焦。
type WindowService struct {
	mu      sync.Mutex
	windows map[string]*application.WebviewWindow
}

// NewWindowService 创建窗口服务。
func NewWindowService() *WindowService {
	return &WindowService{windows: make(map[string]*application.WebviewWindow)}
}

// OpenSettings 打开（或聚焦）设置窗口。
func (s *WindowService) OpenSettings() {
	s.open(settingsWindowName, "/#/settings", settingsWindowW, settingsWindowH)
}

// OpenAddDrive 打开（或聚焦）新增驱动器窗口。
func (s *WindowService) OpenAddDrive() {
	s.open(driveWindowName, "/#/drive", driveWindowW, driveWindowH)
}

// OpenEditDrive 打开（或聚焦）编辑驱动器窗口，name 为现有远程名称。
func (s *WindowService) OpenEditDrive(name string) {
	s.open(driveWindowName, "/#/drive?edit="+url.QueryEscape(name), driveWindowW, driveWindowH)
}

func (s *WindowService) open(name, path string, width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.windows[name]; ok {
		existing.SetURL(path)
		existing.Show()
		existing.Focus()
		return
	}

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
	// 窗口关闭后从缓存移除；下次打开会重新创建（子窗口关闭 = 销毁，不驻留托盘）
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.mu.Lock()
		delete(s.windows, name)
		s.mu.Unlock()
	})
	s.windows[name] = window
	window.Center()
}
