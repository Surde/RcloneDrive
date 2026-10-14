//go:build windows

package main

import (
	"os"
	"path/filepath"
)

// isInstalledBuild 判断当前运行的是否为 NSIS 安装版：
// 安装目录里有 uninstall.exe（NSIS 卸载器），便携版没有。
func isInstalledBuild() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(exe), "uninstall.exe"))
	return err == nil
}
