package main

import (
	"context"
	"fmt"
	"log"
	"maps"
	"time"

	"rclone-drive/internal/rclone"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// autoConnectDelay 自动连接的等待时间：等界面起来再挂载，避免与应用启动抢资源。
const autoConnectDelay = time.Second

// MountEvent 是挂载状态变更事件的负载。
type MountEvent struct {
	Name    string `json:"name"`
	Mounted bool   `json:"mounted"`
	Error   string `json:"error"`
}

// MountStatus 描述单个远程存储的挂载状态。
type MountStatus struct {
	Mounted bool   `json:"mounted"`
	Log     string `json:"log"`
}

// SpeedEvent 是挂载速率事件的负载（远程名 → 上下行速率）。
type SpeedEvent struct {
	Speeds map[string]rclone.Speed `json:"speeds"`
}

func init() {
	application.RegisterEvent[MountEvent]("mount:changed")
	application.RegisterEvent[SpeedEvent]("mount:speed")
}

// MountService 提供挂载/卸载操作并向前端广播状态变更事件。
type MountService struct {
	app     *application.App
	remotes *RemoteService
	mounts  *rclone.MountManager
}

// NewMountService 创建挂载服务。
func NewMountService(remotes *RemoteService, mounts *rclone.MountManager) *MountService {
	return &MountService{remotes: remotes, mounts: mounts}
}

// ServiceStartup 保存应用实例并注册状态变更回调。
func (s *MountService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	s.app = application.Get()
	s.mounts.OnChange(func(name string, mounted bool, errMsg string) {
		if s.app != nil {
			s.app.Event.Emit("mount:changed", MountEvent{Name: name, Mounted: mounted, Error: errMsg})
		}
		// 挂载在成功之后异常结束（连接中断等）时用系统通知提醒
		if !mounted && errMsg != "" {
			notifyUser("error", "已断开连接", "「"+name+"」："+errMsg)
		}
	})
	// 应用已启动，此时刷新托盘菜单
	refreshTray()
	// 启动后台自动连接（标记了「自动连接」的驱动器）
	go s.autoConnect()
	// 有驱动器挂载时持续把速率推给界面
	go s.watchSpeeds()
	return nil
}

// autoConnect 依次挂载所有标记了「自动连接」且当前未挂载的驱动器。
// 单台失败只发通知并继续处理后面的，避免一台配错导致其余都不连。
func (s *MountService) autoConnect() {
	time.Sleep(autoConnectDelay)
	for _, remote := range s.remotes.List() {
		if !remote.AutoConnect || s.mounts.IsMounted(remote.Name) {
			continue
		}
		if err := s.Mount(remote.Name); err != nil {
			notifyUser("error", "自动连接失败", fmt.Sprintf("「%s」：%v", remote.Name, err))
		}
	}
}

// ServiceShutdown 退出时卸载所有挂载。
func (s *MountService) ServiceShutdown() error {
	s.mounts.UnmountAll()
	return nil
}

// Mount 挂载指定远程存储；未设置挂载点时自动选择默认盘符/目录。
func (s *MountService) Mount(name string) error {
	remote, ok := s.remotes.Get(name)
	if !ok {
		return fmt.Errorf("远程存储不存在: %s", name)
	}
	target := remote.MountPoint
	if target == "" {
		target = nextFreeMountPoint()
		if target == "" {
			return fmt.Errorf("没有可用的挂载盘符")
		}
	}
	return s.mounts.Mount(name, target, remote.ReadOnly, remote.Insecure)
}

// Unmount 卸载指定远程存储。
func (s *MountService) Unmount(name string) error {
	return s.mounts.Unmount(name)
}

// OpenFolder 在系统文件管理器中打开已连接驱动器对应的挂载目录。
func (s *MountService) OpenFolder(name string) error {
	if !s.mounts.IsMounted(name) {
		return fmt.Errorf("「%s」未连接，请先连接后再打开文件夹", name)
	}
	target := s.mounts.Target(name)
	if target == "" {
		return fmt.Errorf("驱动器「%s」没有可用的挂载点", name)
	}
	return openDirectory(target)
}

// Status 返回指定远程存储的挂载状态与最近日志。
func (s *MountService) Status(name string) MountStatus {
	return MountStatus{
		Mounted: s.mounts.IsMounted(name),
		Log:     s.mounts.Log(name),
	}
}

// ListStatuses 返回所有远程存储的挂载状态。
func (s *MountService) ListStatuses() []MountStatus {
	statuses := make([]MountStatus, 0)
	for _, remote := range s.remotes.List() {
		status := s.Status(remote.Name)
		statuses = append(statuses, status)
	}
	return statuses
}

// Speeds 返回各已挂载驱动器的实时速率（上下行，字节/秒）。
func (s *MountService) Speeds() map[string]rclone.Speed {
	return s.mounts.Speeds()
}

// Usages 返回各已挂载驱动器的容量信息（后端不支持的不会出现在结果里）。
func (s *MountService) Usages() map[string]rclone.Usage {
	return s.mounts.Usages()
}

// watchSpeeds 每秒采样一次速率，变化时推送给界面。
//
// 由后端推送而不是让界面轮询：浏览器会把后台或被遮挡窗口的定时器强烈节流
// （setInterval 可能被压到每分钟一次），那样复制大文件时速率要等很久才显示。
func (s *MountService) watchSpeeds() {
	// rclone 内部只在 1 秒的 ticker 上重算速率（fs/accounting/accounting.go:218），
	// 所以这里取 250ms：足够及时地把「当前速率」推给界面，又不会做无意义的高频调用
	const interval = 250 * time.Millisecond
	var last map[string]rclone.Speed
	lastActive := -1
	for {
		time.Sleep(interval)
		speeds := s.mounts.Speeds()
		if len(speeds) == 0 && len(last) == 0 {
			continue
		}
		active := 0
		for _, speed := range speeds {
			if speed.Down > 0 || speed.Up > 0 {
				active++
			}
		}
		if active != lastActive {
			log.Printf("[speed] 有流量的驱动器 %d 个", active)
			lastActive = active
		}
		if maps.Equal(speeds, last) {
			continue
		}
		last = speeds
		if s.app == nil {
			continue
		}
		// 全部断开时推一个空表，让界面清掉残留数字
		s.app.Event.Emit("mount:speed", SpeedEvent{Speeds: speeds})
	}
}
