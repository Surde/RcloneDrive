package main

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed build/windows/tray.png
var trayIconPNG []byte

// trayFlag 使程序静默启动到托盘（开机自启项会带上它）。
const trayFlag = "--tray"

// trayManager 负责系统托盘及其菜单；菜单在远程列表或挂载状态变化后重建。
type trayManager struct {
	app     *application.App
	window  *application.WebviewWindow
	remotes *RemoteService
	mounts  *MountService
	windows *WindowService
	menu    *application.Menu

	// pendingMu 保护 pending：正在连接/断开的驱动器，用于显示进行中状态并避免重复触发。
	pendingMu sync.Mutex
	pending   map[string]string
}

// trayInstance 是当前托盘管理器，供服务启动后刷新菜单使用。
var trayInstance *trayManager

// refreshTray 在托盘已创建时重建菜单（应用启动、列表或挂载状态变化时调用）。
func refreshTray() {
	if trayInstance != nil {
		trayInstance.refresh()
	}
}

func newTrayManager(app *application.App, window *application.WebviewWindow, remotes *RemoteService, mounts *MountService, windows *WindowService) *trayManager {
	manager := &trayManager{
		app:     app,
		window:  window,
		remotes: remotes,
		mounts:  mounts,
		windows: windows,
		pending: make(map[string]string),
	}

	tray := app.SystemTray.New()
	tray.SetIcon(trayIconPNG)
	tray.SetTooltip("RcloneDrive")
	manager.menu = app.NewMenu()
	tray.SetMenu(manager.menu)

	// 托盘菜单随挂载状态变化刷新
	mounts.mounts.OnChange(func(string, bool, string) { manager.refresh() })

	trayInstance = manager
	manager.refresh()
	return manager
}

// refresh 重建托盘菜单。
func (t *trayManager) refresh() {
	t.menu.Clear()
	t.menu.Add(fmt.Sprintf("RcloneDrive v%s", appVersion)).SetEnabled(false)
	t.menu.AddSeparator()
	t.menu.Add("显示主界面").OnClick(func(*application.Context) { t.showWindow() })
	t.menu.Add("检查更新").OnClick(func(*application.Context) { go t.handleCheckUpdate() })
	t.menu.Add("添加驱动器").OnClick(func(*application.Context) { t.windows.OpenAddDrive() })
	t.menu.Add("设置").OnClick(func(*application.Context) { t.windows.OpenSettings() })

	remotes := t.remotes.List()
	if len(remotes) > 0 {
		t.menu.AddSeparator()
		for _, remote := range remotes {
			name := remote.Name
			label := name
			if target := t.mountTarget(name, remote.MountPoint); target != "" {
				label = fmt.Sprintf("%s (%s)", name, strings.TrimSuffix(target, ":"))
			}
			sub := t.menu.AddSubmenu(label)

			// 状态行实时反映连接状态，动作项随之切换
			if pending := t.pendingAction(name); pending != "" {
				// 正在连接/断开：只显示进行中的状态，不提供动作项，避免重复触发
				sub.Add(pending).SetEnabled(false)
			} else if t.mounts.mounts.IsMounted(name) {
				sub.Add("已连接").SetEnabled(false)
				sub.Add("断开连接").OnClick(func(*application.Context) { go t.unmount(name) })
			} else {
				sub.Add("未连接").SetEnabled(false)
				sub.Add("开始连接").OnClick(func(*application.Context) { go t.mount(name) })
			}
			sub.Add("编辑").OnClick(func(*application.Context) { t.windows.OpenEditDrive(name) })
		}
	}

	t.menu.AddSeparator()
	t.menu.Add("退出").OnClick(func(*application.Context) { t.app.Quit() })
	t.menu.Update()
}

// mount 在后台挂载（可能耗时数秒，避免阻塞托盘菜单）；结果用系统通知提示。
func (t *trayManager) mount(name string) {
	t.setPending(name, "连接中…")
	err := t.mounts.Mount(name)
	t.clearPending(name)
	if err != nil {
		notifyNative("挂载失败", fmt.Sprintf("「%s」：%v", name, err))
	} else {
		notifyNative("已连接", fmt.Sprintf("「%s」已挂载", name))
	}
	t.refresh()
}

func (t *trayManager) unmount(name string) {
	t.setPending(name, "断开中…")
	err := t.mounts.Unmount(name)
	t.clearPending(name)
	if err != nil {
		notifyNative("断开失败", fmt.Sprintf("「%s」：%v", name, err))
	} else {
		notifyNative("已断开", fmt.Sprintf("「%s」已断开连接", name))
	}
	t.refresh()
}

// mountTarget 返回当前生效的挂载点：已挂载时用实际盘符/目录，否则用驱动器配置里的值。
func (t *trayManager) mountTarget(name, configured string) string {
	if target := t.mounts.mounts.Target(name); target != "" {
		return target
	}
	return configured
}

// setPending 记录进行中的动作并立刻刷新菜单（显示「连接中…」等状态）。
func (t *trayManager) setPending(name, action string) {
	t.pendingMu.Lock()
	t.pending[name] = action
	t.pendingMu.Unlock()
	t.refresh()
}

// clearPending 清除进行中的动作标记（状态变化后由 refresh 重新渲染）。
func (t *trayManager) clearPending(name string) {
	t.pendingMu.Lock()
	delete(t.pending, name)
	t.pendingMu.Unlock()
}

// pendingAction 返回驱动器进行中的动作文字，无进行中动作时返回空串。
func (t *trayManager) pendingAction(name string) string {
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	return t.pending[name]
}

func (t *trayManager) handleCheckUpdate() {
	result := checkUpdate()
	if result.Status == "update" {
		notifyNative("发现新版本 v"+result.Latest, result.URL)
		return
	}
	if result.Status == "error" {
		notifyNative("检查更新失败", result.Message)
		return
	}
	notifyNative("检查更新", result.Message)
}

func (t *trayManager) showWindow() {
	t.window.Show()
	t.window.Focus()
}
