//go:build !windows

package main

import (
	"os"
	"path/filepath"

	"rclonedrive/internal/rclone"
)

// nextFreeMountPoint 返回挂载根目录（<UserConfigDir>/RcloneDrive/mounts）。
func nextFreeMountPoint() string {
	dir := filepath.Join(rclone.ConfigDir(), "mounts")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// defaultMountTarget 返回驱动器的默认挂载点：挂载根目录下以驱动器名命名的
// 独立子目录（<UserConfigDir>/RcloneDrive/mounts/<名称>）。每个驱动器各占一个
// 子目录，避免多个驱动器默认都指向 mounts 根目录、后挂的因目录被占用而失败。
func defaultMountTarget(name string) string {
	return filepath.Join(nextFreeMountPoint(), mountNameDir(name))
}

// usedDriveLetters 在非 Windows 平台上没有盘符概念，返回空。
func usedDriveLetters() []string {
	return nil
}
