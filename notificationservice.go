package main

import "github.com/wailsapp/wails/v3/pkg/application"

// ToastEvent 是发给界面的应用内提示；主窗口可见时用它替代系统通知。
type ToastEvent struct {
	Level string `json:"level"` // success / error
	Title string `json:"title"`
	Body  string `json:"body"`
}

const toastEvent = "app:toast"

func init() {
	application.RegisterEvent[ToastEvent](toastEvent)
}

// mainWindow 是主窗口，用于判断界面是否正在被看到（见 notifyUser）。
var mainWindow *application.WebviewWindow

// notifyUser 提示用户：主窗口可见时发应用内 toast，隐藏到托盘时改用系统通知
// （窗口藏着的时候 toast 看不到，那条提示就白发了）。
func notifyUser(level, title, body string) {
	if mainWindow != nil && mainWindow.IsVisible() {
		if app := application.Get(); app != nil {
			app.Event.Emit(toastEvent, ToastEvent{Level: level, Title: title, Body: body})
			return
		}
	}
	notifyNative(title, body)
}
