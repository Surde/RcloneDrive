package main

import (
	"runtime"

	"rclone-drive/internal/rclone"

	"github.com/rclone/rclone/fs"
)

// RcloneStatus 描述内嵌 rclone 引擎的状态。
type RcloneStatus struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

// SystemService 提供系统级的查询能力。
type SystemService struct{}

// RcloneStatus 报告内嵌 rclone 引擎的版本与挂载能力。
func (s *SystemService) RcloneStatus() RcloneStatus {
	engine := rclone.MountType()
	if engine == "" {
		return RcloneStatus{}
	}
	return RcloneStatus{
		Found:   true,
		Path:    "内嵌 rclone 引擎 (" + engine + ")",
		Version: fs.Version,
	}
}

// Platform 返回当前操作系统（windows / darwin / linux ...）。
func (s *SystemService) Platform() string {
	return runtime.GOOS
}

// DefaultMountPoint 返回默认挂载点（Windows 为空闲盘符，其他系统为挂载目录）。
func (s *SystemService) DefaultMountPoint() string {
	return nextFreeMountPoint()
}

// UsedDriveLetters 返回已被占用的盘符（如 ["C:", "Y:"]），供界面过滤盘符下拉框；
// 非 Windows 平台没有盘符概念，返回空列表。
func (s *SystemService) UsedDriveLetters() []string {
	return usedDriveLetters()
}
