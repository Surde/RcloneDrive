package main

import (
	"context"
	"fmt"

	"rclone-drive/internal/rclone"

	"github.com/wailsapp/wails/v3/pkg/application"
)

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

func init() {
	application.RegisterEvent[MountEvent]("mount:changed")
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
			notifyNative("已断开连接", "「"+name+"」："+errMsg)
		}
	})
	// 应用已启动，此时刷新托盘菜单
	refreshTray()
	return nil
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
	return s.mounts.Mount(name, target, remote.ReadOnly)
}

// Unmount 卸载指定远程存储。
func (s *MountService) Unmount(name string) error {
	return s.mounts.Unmount(name)
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
