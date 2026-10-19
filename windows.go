package main

import (
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 子窗口的名称与尺寸（沿用旧版 windowservice.go 的 WebviewWindowOptions：
// 无边框固定尺寸，标题栏由视图的 WindowBar 自绘）。
const (
	settingsWindowName = "settings"
	settingsWindowW    = 520
	settingsWindowH    = 520
	driveWindowName    = "drive"
	driveWindowW       = 760
	driveWindowH       = 680
)

// windowManager 负责打开设置、新增/编辑驱动器等独立窗口；已打开时复用并聚焦。
// mygo 只有 WindowByID、没有 GetByName，窗口按名字存在 map 里自己管理。
// 每个窗口对应一个视图状态结构（settingsViewState / driveEditorState），
// 复用时状态结构已切到新目标，Invalidate 重绘即可（替代旧版 drive:open 事件）。
type windowManager struct {
	mu      sync.Mutex
	windows map[string]*mygo.Window
	// main 是主窗口，用于子窗口关闭后把主界面带到前台
	main *mygo.Window
	// driveAddMode 记录驱动器窗口当前是否为「新增」模式（关闭后要显示主界面）
	driveAddMode bool
	// quitting 为 true 时各窗口的 OnClose 不再拦截关闭（默认拦截=隐藏到托盘）。
	// App.Quit 会逐个关闭窗口，任何 PreventDefault 都会中止退出，
	// 所以真正退出前必须先置位（见 quit）。
	quitting atomic.Bool

	settingsState *settingsViewState
	driveState    *driveEditorState
}

// allWindows 是全局窗口管理器，供视图与 notificationservice.go 使用。
var allWindows *windowManager

func newWindowManager(main *mygo.Window) *windowManager {
	m := &windowManager{
		windows:       make(map[string]*mygo.Window),
		main:          main,
		settingsState: &settingsViewState{},
		driveState:    newDriveEditorState(),
	}
	allWindows = m
	mainWindow = main
	return m
}

// find 按名称查找已登记的窗口。
func (s *windowManager) find(name string) *mygo.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windows[name]
}

// hide 隐藏指定窗口（视图 WindowBar 的关闭按钮；与 OnClose 拦截等效）。
// 主窗口不在 windows map 里（它是 newWindowManager 的 main 字段），单独处理。
func (s *windowManager) hide(name string) {
	w := s.find(name)
	if w == nil && name == "main" {
		w = s.main
	}
	if w != nil {
		w.Hide()
	}
}

// quit 发起真正退出（托盘「退出」）：先置 quitting 让各窗口的
// OnClose 放行关闭，再调 App.Quit——它会逐个关闭窗口，
// 被 PreventDefault 拦截会中止退出。
func (s *windowManager) quit() {
	s.quitting.Store(true)
	mygo.App.Quit()
}

// showMainWindow 显示并聚焦主窗口。
func (s *windowManager) showMainWindow() {
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
func (s *windowManager) OpenSettings() {
	if s.reuse(settingsWindowName, nil) {
		return
	}
	s.create(settingsWindowName, settingsWindowW, settingsWindowH, ui.View(s.settingsState.view))
}

// OpenAddDrive 打开（或聚焦）新增驱动器窗口。
func (s *windowManager) OpenAddDrive() {
	s.openDriveWindow("")
}

// OpenEditDrive 打开（或聚焦）编辑驱动器窗口，name 为现有远程名称。
// 驱动器已连接时的确认与断开由卡片上的确认对话框负责（见 mainview.go）。
func (s *windowManager) OpenEditDrive(name string) {
	s.openDriveWindow(name)
}

// openDriveWindow 打开驱动器窗口；窗口已创建时切到目标内容并复用。
// 旧版靠 drive:open 事件让复用的窗口切换内容（新建 WebView2 窗口要 1 秒以上）；
// native 模式的视图状态直接换成新目标，Invalidate 重绘即可。
func (s *windowManager) openDriveWindow(name string) {
	addMode := name == ""
	s.mu.Lock()
	s.driveAddMode = addMode
	s.mu.Unlock()

	s.driveState.setTarget(name)
	if s.reuse(driveWindowName, func(w *mygo.Window) { w.Invalidate() }) {
		return
	}
	s.create(driveWindowName, driveWindowW, driveWindowH, ui.View(s.driveState.view))
	s.driveState.win = s.find(driveWindowName) // 后台任务经 Window.Update 回主线程改状态
}

// reuse 复用已创建的窗口：必要时先执行 switchTo，然后显示并聚焦。
func (s *windowManager) reuse(name string, switchTo func(*mygo.Window)) bool {
	s.mu.Lock()
	window := s.windows[name]
	s.mu.Unlock()
	if window == nil {
		return false
	}
	if switchTo != nil {
		switchTo(window)
	}
	window.Show()
	window.Focus()
	return true
}

func (s *windowManager) create(name string, width, height int, content *ui.Content) {
	s.mu.Lock()
	defer s.mu.Unlock()

	window := mygo.NewWindow(mygo.WindowOptions{
		Title:         "RcloneDrive",
		Width:         width,
		Height:        height,
		DisableResize: true,
		Frameless:     true,
		Content:       content,
	})
	// 关闭只隐藏：窗口与其状态保持存活，下次打开直接复用（见 reuse）。
	// 新增驱动器窗口关闭后（取消或保存）把主界面带到前台。
	// quitting 时不拦截——App.Quit 正在逐个关闭窗口（见 quit）。
	window.OnClose(func(e *mygo.CloseEvent) {
		if s.quitting.Load() {
			return
		}
		e.PreventDefault()
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

// invalidateAll 让所有已登记窗口重绘（服务状态变化后调用）。
func (s *windowManager) invalidateAll() {
	s.mu.Lock()
	windows := make([]*mygo.Window, 0, len(s.windows)+1)
	for _, w := range s.windows {
		windows = append(windows, w)
	}
	if s.main != nil {
		windows = append(windows, s.main)
	}
	s.mu.Unlock()
	for _, w := range windows {
		w.Invalidate()
	}
}
