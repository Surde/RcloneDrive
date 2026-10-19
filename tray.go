package main

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo"
)

//go:embed resources/tray.png
var trayIconPNG []byte

// appIconPNG 是应用图标（resources/appicon.png）。mygo dev/build 会把它嵌成
// exe/window 图标；裸 go build 的 exe 没有图标资源，运行时经 Dock.SetIcon
// 设置任务栏图标（Windows 上作用于全部窗口）。
//
//go:embed resources/appicon.png
var appIconPNG []byte

// trayFlag 使程序静默启动到托盘（开机自启项会带上它）。
const trayFlag = "--tray"

// trayManager 负责系统托盘及其菜单；菜单在远程列表或挂载状态变化后重建。
// mygo 的菜单在 SetMenu 时做原生快照，重建就是生成新的 *mygo.Menu 再 SetMenu。
type trayManager struct {
	tray    *mygo.Tray
	windows *windowManager
	remotes *RemoteService
	mounts  *MountService
	updates *UpdateService

	// pendingMu 保护 pending：正在连接/断开的驱动器，用于显示进行中状态并避免重复触发。
	pendingMu sync.Mutex
	pending   map[string]string
}

// newTrayManager 创建托盘。必须在 App.WhenReady 里调用（mygo 要求托盘在应用就绪后创建）。
// 挂载状态与远程列表变化时重建菜单；初始菜单在订阅后重建一次。
func newTrayManager(windows *windowManager, remotes *RemoteService, mounts *MountService, updates *UpdateService) (*trayManager, error) {
	manager := &trayManager{
		windows: windows,
		remotes: remotes,
		mounts:  mounts,
		updates: updates,
		pending: make(map[string]string),
	}

	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    trayIconPNG,
		ToolTip: "RcloneDrive",
		Menu:    manager.buildMenu(),
	})
	if err != nil {
		return nil, err
	}
	manager.tray = tray

	// 单击托盘图标聚焦主窗口（近似旧版的双击语义；Windows 上菜单存在时
	// 左键仍触发 OnClick，右键弹出下面设置的菜单）
	tray.OnClick(manager.showWindow)

	// 托盘菜单随挂载状态与远程列表变化刷新
	mounts.mounts.OnChange(func(string, bool, string) { manager.refresh() })
	remotes.OnChange(manager.refresh)

	return manager, nil
}

// buildMenu 按当前状态生成一份新的托盘菜单。
func (t *trayManager) buildMenu() *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: fmt.Sprintf("RcloneDrive v%s", appVersion), Disabled: true},
		mygo.Separator(),
		{Label: "显示主界面", Click: func(*mygo.MenuItem, *mygo.Window) { t.showWindow() }},
		{Label: "检查更新", Click: func(*mygo.MenuItem, *mygo.Window) { go t.updates.checkFromTray() }},
		{Label: "添加驱动器", Click: func(*mygo.MenuItem, *mygo.Window) { t.windows.OpenAddDrive() }},
		{Label: "设置", Click: func(*mygo.MenuItem, *mygo.Window) { t.windows.OpenSettings() }},
	}

	remotes := t.remotes.List()
	if len(remotes) > 0 {
		items = append(items, mygo.Separator())
		for _, remote := range remotes {
			items = append(items, t.remoteItem(remote))
		}
	}

	items = append(items,
		mygo.Separator(),
		// 退出：先卸载全部挂载，再真正退出应用（先放行 OnClose 拦截）
		&mygo.MenuItem{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) {
			t.mounts.Stop()
			t.windows.quit()
		}},
	)
	return mygo.NewMenu(items)
}

// remoteItem 生成一个远程存储的子菜单：状态行 + 连接/断开 + 编辑。
func (t *trayManager) remoteItem(remote Remote) *mygo.MenuItem {
	name := remote.Name
	label := name
	if target := t.mountTarget(name, remote.MountPoint); target != "" {
		label = fmt.Sprintf("%s (%s)", name, strings.TrimSuffix(target, ":"))
	}

	sub := []*mygo.MenuItem{}
	// 状态行实时反映连接状态，动作项随之切换
	if pending := t.pendingAction(name); pending != "" {
		// 正在连接/断开：只显示进行中的状态，不提供动作项，避免重复触发
		sub = append(sub, &mygo.MenuItem{Label: pending, Disabled: true})
	} else if t.mounts.mounts.IsMounted(name) {
		sub = append(sub,
			&mygo.MenuItem{Label: "已连接", Disabled: true},
			&mygo.MenuItem{Label: "断开连接", Click: func(*mygo.MenuItem, *mygo.Window) { go t.unmount(name) }},
		)
	} else {
		sub = append(sub,
			&mygo.MenuItem{Label: "未连接", Disabled: true},
			&mygo.MenuItem{Label: "开始连接", Click: func(*mygo.MenuItem, *mygo.Window) { go t.mount(name) }},
		)
	}
	sub = append(sub, &mygo.MenuItem{Label: "编辑", Click: func(*mygo.MenuItem, *mygo.Window) { t.editDrive(name) }})
	return &mygo.MenuItem{Label: label, Submenu: sub}
}

// refresh 重建托盘菜单。mygo 的菜单项回调与弹出都走主线程：菜单弹出期间
// （trackPopup 阻塞）其他线程发来的重建请求会在弹出关闭后才执行，
// 天然避免了旧版 Wails 里「重建销毁正在显示的原生菜单」的竞态，无需再延迟。
func (t *trayManager) refresh() {
	if t.tray == nil {
		return
	}
	t.tray.SetMenu(t.buildMenu())
}

// mount 在后台挂载（可能耗时数秒，避免阻塞托盘菜单）。
// 连接/断开不在窗口弹提示（卡片状态与托盘菜单已反馈），只发系统通知。
func (t *trayManager) mount(name string) {
	t.setPending(name, "连接中…")
	err := t.mounts.Mount(name)
	t.clearPending(name)
	if err != nil {
		notifyNativeOnly("挂载失败", fmt.Sprintf("「%s」：%v", name, err))
	} else {
		notifyNativeOnly("已连接", fmt.Sprintf("「%s」已挂载", name))
	}
	t.refresh()
}

func (t *trayManager) unmount(name string) {
	t.setPending(name, "断开中…")
	err := t.mounts.Unmount(name)
	t.clearPending(name)
	if err != nil {
		notifyNativeOnly("断开失败", fmt.Sprintf("「%s」：%v", name, err))
	} else {
		notifyNativeOnly("已断开", fmt.Sprintf("「%s」已断开连接", name))
	}
	t.refresh()
}

// editDrive 打开驱动器的编辑窗口；已连接的驱动器必须先断开连接（界面确认框负责断开）。
func (t *trayManager) editDrive(name string) {
	if t.mounts.mounts.IsMounted(name) {
		notifyUser("error", "无法编辑", fmt.Sprintf("「%s」已连接，请先断开连接再编辑", name))
		return
	}
	t.windows.OpenEditDrive(name)
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

func (t *trayManager) showWindow() {
	t.windows.showMainWindow()
}
