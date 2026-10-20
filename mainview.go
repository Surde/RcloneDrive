package main

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"rclonedrive/internal/rclone"

	"github.com/egoist/mygo/ui"
)

// mainWindowView 是主窗口的视图状态（对应旧前端 App.vue + DriveCard.vue）：
// 自绘 WindowBar、驱动卡片列表（状态/速率/容量）、更新提示条、空状态。
// 数据每帧从服务查询（服务回调已触发 invalidateAll，见 main.go 接线）。
var mainWindowView = &mainViewState{}

type mainViewState struct {
	// busy 记录正在连接/断开的驱动器，避免重复触发
	busyMu sync.Mutex
	busy   map[string]bool

	// 确认对话框：一次只开一个（AlertDialog 需要 *bool 状态）
	editConfirm    bool
	editTarget     string
	deleteConfirm  bool
	deleteTarget   string
	restartConfirm bool

	// rclone 引擎状态（一次性加载，内存查询很快）
	rcloneLoaded bool
	rcloneOK     bool

	// 容量信息变化很慢（旧前端每 60s 拉一次）：后台 goroutine 缓存
	usageMu  sync.Mutex
	usages   map[string]rclone.Usage
	usageKey string    // 已挂载集合的指纹，变化时重新拉取
	usageAt  time.Time // 上次拉取时间
}

func (s *mainViewState) view(c *ui.Context) {
	t := applyAppearance(c)
	if s.busy == nil {
		s.busy = make(map[string]bool)
	}

	snapshot := s.snapshot()

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		windowBar(c, t, "RcloneDrive", "", func() {
			iconButton(c, t, "添加驱动器", iconPlus, func() {
				if allWindows != nil {
					allWindows.OpenAddDrive()
				}
			})
			iconButton(c, t, "设置", iconSettings, func() {
				if allWindows != nil {
					allWindows.OpenSettings()
				}
			})
		}, func() {
			if allWindows != nil {
				allWindows.hide("main")
			}
		})

		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(12).Gap(10).Children(func() {
				s.rcloneAlert(c, t)
				s.updateBanner(c, t)
				if len(snapshot.remotes) == 0 {
					s.emptyState(c, t)
				} else {
					for _, remote := range snapshot.remotes {
						s.driveCard(c, t, remote, snapshot)
					}
				}
			})
		})
	})

	drainToasts(c, "main")
}

// mainSnapshot 是一帧内用到的服务数据快照。
type mainSnapshot struct {
	remotes  []Remote
	statuses map[string]MountStatus
	speeds   map[string]rclone.Speed
	labels   map[string]string
}

func (s *mainViewState) snapshot() mainSnapshot {
	out := mainSnapshot{
		remotes:  remoteService.List(),
		statuses: make(map[string]MountStatus),
		speeds:   mountService.Speeds(),
		labels:   remoteService.TypeLabels(),
	}
	// Status 是内存查询，逐驱动器取一次（数量小）
	for _, r := range out.remotes {
		out.statuses[r.Name] = mountService.Status(r.Name)
	}
	s.refreshUsagesAsync(out.statuses)
	return out
}

// refreshUsagesAsync 在挂载集合变化或距上次超过 60s 时后台拉取容量信息。
// About 查询可能走网络（sftp 的 df），不能阻塞主线程的帧构建。
func (s *mainViewState) refreshUsagesAsync(statuses map[string]MountStatus) {
	key := ""
	for name, st := range statuses {
		if st.Mounted {
			key += name + "\x00"
		}
	}
	s.usageMu.Lock()
	unchanged := key == s.usageKey && len(key) > 0 && time.Since(s.usageAt) < usageRefreshInterval
	if len(key) == 0 {
		s.usages, s.usageKey, s.usageAt = map[string]rclone.Usage{}, "", time.Now()
		s.usageMu.Unlock()
		return
	}
	s.usageMu.Unlock()
	if unchanged {
		return
	}
	go func() {
		usages := mountService.Usages()
		s.usageMu.Lock()
		s.usages, s.usageKey, s.usageAt = usages, key, time.Now()
		s.usageMu.Unlock()
		if allWindows != nil {
			allWindows.invalidateAll()
		}
	}()
}

// usageRefreshInterval 对应旧前端 useDrives 的 USAGE_REFRESH_MS。
const usageRefreshInterval = 60 * time.Second

// rcloneAlert 在当前构建未启用挂载（无 cmount 标签）时显示警告条。
func (s *mainViewState) rcloneAlert(c *ui.Context, t *ui.Theme) {
	if !s.rcloneLoaded {
		s.rcloneLoaded = true
		s.rcloneOK = systemService.RcloneStatus().Found
	}
	if s.rcloneOK {
		return
	}
	ui.Row(c).FillWidth().Gap(8).Padding(10, 12).Radius(t.Radius).
		Border(1, t.Warning).Children(func() {
		ui.Icon(c, iconWarning).Size(14, 14).TextColor(t.Warning)
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, "当前构建未启用挂载功能").Bold()
			ui.Text(c, rcloneAlertHelp()).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		})
	})
}

// rcloneAlertHelp 返回平台相关的挂载启用提示（只提当前平台的要求）。
func rcloneAlertHelp() string {
	switch runtime.GOOS {
	case "windows":
		return "挂载依赖内嵌 rclone 引擎的 cmount 实现，需要以 cmount 构建标签编译；Windows 上还需安装 WinFsp（安装包已内嵌，首次挂载会自动安装）。"
	case "darwin":
		return "挂载依赖内嵌 rclone 引擎的 cmount 实现，需要以 cmount 构建标签编译，并在系统中安装 macFUSE。"
	default:
		return "挂载依赖内嵌 rclone 引擎的 cmount 实现，需要以 cmount 构建标签编译（启用 cgo），并在系统中安装 fuse3。"
	}
}

// updateBanner 是更新提示条（检查中/下载中/就绪）。旧前端没有主窗口横幅
// （更新 UI 在设置窗口），这里按任务要求补上：状态来自 UpdateService.Status()。
func (s *mainViewState) updateBanner(c *ui.Context, t *ui.Theme) {
	if updateService == nil {
		return
	}
	status := updateService.Status()
	switch status.State {
	case stateChecking:
		s.bannerRow(c, t, t.Accent, "正在检查更新…", nil)
	case stateAvailable:
		s.bannerRow(c, t, t.Accent, "发现新版本 v"+status.Latest+"，准备下载…", nil)
	case stateDownloading, stateVerifying, stateInstalling:
		progress := updateService.Progress()
		percent := 0
		if progress.Total > 0 {
			percent = int(progress.Written * 100 / progress.Total)
		}
		s.bannerRow(c, t, t.Accent, "", func() {
			ui.Column(c).Gap(4).Grow(1).Children(func() {
				ui.Text(c, "正在下载更新…").Bold()
				ui.Progress(c, float64(percent)/100).FillWidth()
				ui.Textf(c, "%d%% · %s / %s", percent, formatSize(float64(progress.Written)), formatSize(float64(progress.Total))).
					FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			})
		})
	case stateReady:
		if status.Downloaded && status.Verified {
			s.bannerRow(c, t, t.Success, "更新已就绪（校验通过），重启后生效", func() {
				if ui.PrimaryButton(c, "立即重启").Clicked() {
					s.restartConfirm = true
				}
			})
		}
	case stateError:
		s.bannerRow(c, t, t.Danger, "检查更新失败", func() {
			if ui.Button(c, "重试").Clicked() {
				go updateService.CheckManual()
			}
		})
	}

	if s.restartConfirm {
		if ui.AlertDialog(c, &s.restartConfirm, "重启以完成更新？",
			"重启期间所有云盘挂载会临时断开，重启完成后请重新连接。", "取消", "重启") == 1 {
			go func() {
				if err := updateService.Restart(); err != nil {
					notifyUser("error", "重启失败", err.Error())
				}
			}()
		}
	}
}

func (s *mainViewState) bannerRow(c *ui.Context, t *ui.Theme, accent ui.Color, message string, extra func()) {
	ui.Row(c).FillWidth().Gap(8).Padding(10, 12).Radius(t.Radius).AlignItems(ui.Center).
		Border(1, accent).Children(func() {
		if message != "" {
			ui.Text(c, message).Grow(1)
		}
		if extra != nil {
			extra()
		}
	})
}

// emptyState 对应旧前端 Empty：还没有虚拟驱动器。
func (s *mainViewState) emptyState(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillWidth().Gap(8).Padding(32).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconHardDrive).Size(28, 28).TextColor(t.TextMuted)
		ui.Text(c, "还没有虚拟驱动器").Bold().FontSize(t.FontSize + 2)
		ui.Text(c, "点击右上角「添加驱动器」创建你的第一个远程存储驱动器。").TextColor(t.TextMuted)
		if ui.PrimaryButton(c, "添加驱动器").Clicked() {
			if allWindows != nil {
				allWindows.OpenAddDrive()
			}
		}
	})
}

// isDriveLetter 报告挂载点是否是 Windows 盘符形式（如 "Z:"）。
func isDriveLetter(s string) bool {
	return len(s) == 2 && s[1] == ':' && s[0] >= 'A' && s[0] <= 'Z'
}

// driveCard 对应旧前端 DriveCard.vue：协议图标、盘符、名称、类型、
// 状态、容量用量条、实时速率、操作按钮（打开文件夹/挂载/编辑/删除）。
func (s *mainViewState) driveCard(c *ui.Context, t *ui.Theme, remote Remote, snap mainSnapshot) {
	name := remote.Name
	status := snap.statuses[name]
	speed := snap.speeds[name]
	s.usageMu.Lock()
	usage, hasUsage := s.usages[name]
	s.usageMu.Unlock()
	mounted := status.Mounted

	ui.Row(c).FillWidth().Gap(10).Padding(10, 12).Radius(t.Radius).AlignItems(ui.Center).
		Border(1, t.Border).
		Children(func() {
			// 协议图标（圆形底，对应旧 bg-muted rounded-full）。
			// 用 Row 而非 Box：Box 的 Justify 默认 Start，图标只水平居中、
			// 垂直贴顶；Row 双轴居中才是正的。
			ui.Row(c).Size(34, 34).Radius(17).Background(t.Surface).
				Justify(ui.Center).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, protocolIcon(remote.Type)).Size(15, 15).TextColor(t.TextMuted)
			})

			ui.Column(c).Gap(4).Grow(1).Shrink(1).Children(func() {
				// 第 1 行：盘符 + 名称 + 协议
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					// 盘符徽章只展示 Windows 盘符；Linux/macOS 的挂载目录路径
					// 太长不适合徽章，省略。
					if isDriveLetter(remote.MountPoint) {
						// 字号比名称小一档，灰底描边（Badge 字号偏大且不可调）。
						// 展示去掉冒号（Z: → Z），存储仍保留冒号供 rclone 使用。
						ui.Row(c).Padding(1, 6).Radius(t.Radius).Background(t.Surface).
							Border(1, t.Border).Shrink(0).Children(func() {
							ui.Text(c, strings.TrimSuffix(remote.MountPoint, ":")).FontSize(t.FontSize - 2).Bold()
						})
					}
					ui.Text(c, remote.Name).Bold().MaxLines(1).Shrink(1)
					label := snap.labels[remote.Type]
					if label == "" {
						label = remote.Type
					}
					ui.Text(c, label).FontSize(t.FontSize - 2).TextColor(t.TextMuted).MaxLines(1)
				})
				// 第 2 行：状态 + 容量 + 速率
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if mounted {
						ui.Text(c, "已挂载").FontSize(t.FontSize - 2).TextColor(t.Success)
					} else {
						ui.Text(c, "未连接").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
					}
					if mounted && hasUsage && usage.Total > 0 {
						usedPercent := float64(usage.Used) / float64(usage.Total)
						if usedPercent < 0 {
							usedPercent = 0
						}
						if usedPercent > 1 {
							usedPercent = 1
						}
						ui.Box(c).Height(6).Grow(1).Radius(3).Background(t.Surface).Children(func() {
							ui.Box(c).HeightPercent(100).WidthPercent(float32(usedPercent * 100)).Radius(3).Background(t.Accent)
						})
						ui.Textf(c, "%s 可用，共 %s", formatSize(float64(usage.Free)), formatSize(float64(usage.Total))).
							FontSize(t.FontSize - 2).TextColor(t.TextMuted).MaxLines(1)
					}
					if mounted && (speed.Down > 0 || speed.Up > 0) {
						ui.Column(c).Gap(0).Children(func() {
							if speed.Down > 0 {
								ui.Textf(c, "↓ %s", formatSpeed(speed.Down)).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
							}
							if speed.Up > 0 {
								ui.Textf(c, "↑ %s", formatSpeed(speed.Up)).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
							}
						})
					}
				})
			})

			// 操作按钮组
			if mounted {
				iconButton(c, t, "打开文件夹", iconFolderOpen, func() {
					s.runAction(name, func() error { return mountService.OpenFolder(name) }, "打开文件夹失败")
				})
			}
			toggleIcon, tooltip := iconPlug, "挂载"
			if mounted {
				toggleIcon, tooltip = iconUnplug, "断开"
			}
			failTitle := "挂载失败"
			if mounted {
				failTitle = "断开失败"
			}
			iconButton(c, t, tooltip, toggleIcon, func() {
				s.runAction(name, func() error {
					if mounted {
						return mountService.Unmount(name)
					}
					return mountService.Mount(name)
				}, failTitle)
			})
			editTooltip := "编辑"
			if mounted {
				editTooltip = "编辑（需先断开连接）"
			}
			iconButton(c, t, editTooltip, iconPencil, func() {
				if mounted {
					s.editTarget = name
					s.editConfirm = true
				} else if allWindows != nil {
					allWindows.OpenEditDrive(name)
				}
			})
			iconButton(c, t, "删除", iconTrash, func() {
				s.deleteTarget = name
				s.deleteConfirm = true
			})
		})

	// 编辑已连接驱动器：先确认断开（对应旧 DriveCard 的 editConfirmOpen）
	if s.editConfirm && s.editTarget == name {
		if ui.AlertDialog(c, &s.editConfirm, "编辑已连接的驱动器",
			"「"+name+"」当前已连接，编辑前需要先断开连接。是否断开连接并打开编辑窗口？",
			"取消", "断开并编辑") == 1 {
			target := name
			go func() {
				if err := mountService.Unmount(target); err != nil {
					notifyUser("error", "断开失败", err.Error())
					return
				}
				if allWindows != nil {
					allWindows.OpenEditDrive(target)
				}
			}()
		}
	}
	// 删除确认
	if s.deleteConfirm && s.deleteTarget == name {
		if ui.AlertDialog(c, &s.deleteConfirm, "删除驱动器",
			"确定要删除「"+name+"」吗？相关的远程存储配置将被移除。",
			"取消", "删除") == 1 {
			target := name
			go func() {
				if err := remoteService.Delete(target); err != nil {
					notifyUser("error", "删除失败", err.Error())
				}
			}()
		}
	}
}

// runAction 在后台执行连接/断开等动作：进行中禁用按钮，失败发系统通知
// （成功不打扰——卡片状态与托盘菜单已给出反馈，对应旧前端的注释）。
func (s *mainViewState) runAction(name string, action func() error, failTitle string) {
	if s.isBusy(name) {
		return
	}
	s.setBusy(name, true)
	go func() {
		defer s.setBusy(name, false)
		if err := action(); err != nil && failTitle != "" {
			notifyUser("error", failTitle, err.Error())
		}
	}()
}

func (s *mainViewState) isBusy(name string) bool {
	s.busyMu.Lock()
	defer s.busyMu.Unlock()
	return s.busy[name]
}

func (s *mainViewState) setBusy(name string, busy bool) {
	s.busyMu.Lock()
	if busy {
		s.busy[name] = true
	} else {
		delete(s.busy, name)
	}
	s.busyMu.Unlock()
	if allWindows != nil {
		allWindows.invalidateAll()
	}
}
