package main

import "github.com/wailsapp/wails/v3/pkg/application"

// ToastEvent 是发给界面的应用内提示；主窗口可见时用它替代系统通知。
// Target 是提示应出现的窗口名（main / settings / drive）：Wails 的自定义事件
// 会广播到所有窗口，由前端按 Target 过滤，保证提示出现在发起操作的窗口上。
type ToastEvent struct {
	Level  string `json:"level"` // success / error
	Title  string `json:"title"`
	Body   string `json:"body"`
	Target string `json:"target"`
}

const toastEvent = "app:toast"

func init() {
	application.RegisterEvent[ToastEvent](toastEvent)
}

// mainWindow 是主窗口，用于判断界面是否正在被看到（见 notifyUser）。
var mainWindow *application.WebviewWindow

// windowByName 按名称查找窗口；主窗口兼容旧的无名引用。
func windowByName(name string) *application.WebviewWindow {
	if name == "main" && mainWindow != nil {
		return mainWindow
	}
	app := application.Get()
	if app == nil {
		return nil
	}
	w, ok := app.Window.GetByName(name)
	if !ok {
		return nil
	}
	ww, _ := w.(*application.WebviewWindow)
	return ww
}

// notifyUser 提示用户：主窗口可见时发应用内 toast，隐藏到托盘时改用系统通知
// （窗口藏着的时候 toast 看不到，那条提示就白发了）。
func notifyUser(level, title, body string) {
	notifyUserTo("main", level, title, body)
}

// notifyUserTo 与 notifyUser 相同，但 toast 出现在 target 指定的窗口上
// （该窗口不可见时同样回落到系统通知）。target 为空按主窗口处理。
func notifyUserTo(target, level, title, body string) {
	if target == "" {
		target = "main"
	}
	if window := windowByName(target); window != nil && window.IsVisible() {
		if app := application.Get(); app != nil {
			app.Event.Emit(toastEvent, ToastEvent{Level: level, Title: title, Body: body, Target: target})
			return
		}
	}
	notifyNative(title, body)
}

// notifyNativeOnly 只发系统通知，不在任何窗口弹 toast。
// 连接/断开这类动作本身有界面状态反馈（卡片状态、托盘菜单），不需要窗口提示。
func notifyNativeOnly(title, body string) {
	notifyNative(title, body)
}
