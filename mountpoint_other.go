//go:build !windows

package main

import (
	"os"
	"path/filepath"

	"rclone-drive/internal/rclone"
)

// nextFreeMountPoint 返回挂载根目录（<UserConfigDir>/RcloneDrive/mounts）。
func nextFreeMountPoint() string {
	dir := filepath.Join(rclone.ConfigDir(), "mounts")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// usedDriveLetters 在非 Windows 平台上没有盘符概念，返回空。
func usedDriveLetters() []string {
	return nil
}
