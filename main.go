package main

import (
	"embed"
	"log"
	"os"
	"slices"

	"rclone-drive/internal/rclone"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := rclone.Ensure(); err != nil {
		log.Printf("rclone 配置初始化失败: %v", err)
	}
	mountManager := rclone.NewMountManager()
	remoteService := NewRemoteService(mountManager)
	mountService := NewMountService(remoteService, mountManager)
	windowService := NewWindowService()
	settingsService := &SettingsService{}
	updateService := NewUpdateService(settingsService)

	// --tray：开机自启时静默启动到托盘，不弹出窗口
	startHidden := slices.Contains(os.Args[1:], trayFlag)

	app := application.New(application.Options{
		Name:        "RcloneDrive",
		Description: "Mount cloud storage as local drives via rclone",
		Services: []application.Service{
			application.NewService(remoteService),
			application.NewService(mountService),
			application.NewService(&SystemService{}),
			application.NewService(settingsService),
			application.NewService(windowService),
			application.NewService(updateService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// 单实例：重复启动时新进程只把请求交给已有实例（见 singleinstance.go）。
		// 开机自启（--tray）重复启动不打扰用户，直接静默退出。
		SingleInstance: singleInstanceOptions(func(data application.SecondInstanceData) {
			if hasTrayFlag(data.Args) {
				return
			}
			requestShowWindow()
		}),
		// 不设置 Mac.ApplicationShouldTerminateAfterLastWindowClosed：
		// 窗口关闭后程序继续在后台运行（托盘模式）
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "RcloneDrive",
		Name:   "main",
		Width:  580,
		Height: 540,
		// 固定尺寸、无系统窗口边框，标题栏由前端自绘（仅保留关闭按钮）
		DisableResize:    true,
		Frameless:        true,
		Hidden:           startHidden,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	// 关闭窗口 = 隐藏到托盘，挂载保持运行
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
	})

	// 提示走 toast 还是系统通知，取决于主窗口是否可见（见 notifyUser）
	mainWindow = window

	trayHolder := newTrayManager(app, window, remoteService, mountService, windowService, updateService)
	remoteService.OnChange(trayHolder.refresh)
	// 子窗口收尾时需要唤起主窗口（新增驱动器关闭后显示主界面）
	windowService.setMainWindow(window)

	// 第二个实例的显示请求在应用就绪后弹出主界面
	watchSecondInstanceShow(app, trayHolder.showWindow)

	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
