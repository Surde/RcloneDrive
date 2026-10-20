//go:build linux

package main

import (
	"fmt"
	"log"
	"os/exec"
)

// ensureFuse3 挂载前确保 FUSE3 可用：fusermount3 在 PATH 里即认为已安装
// （它由 fuse3 包提供，是挂载的必经入口）。缺失时不静默代装（需要 root），
// 返回带各发行版安装命令的错误，由界面/托盘提示用户。
func ensureFuse3() error {
	if _, err := exec.LookPath("fusermount3"); err == nil {
		return nil
	}
	log.Printf("未检测到 fusermount3（fuse3 未安装）")
	return fmt.Errorf("挂载需要 FUSE3，当前系统未安装，请先安装：\n%s", fuse3InstallHint())
}

// fuse3InstallHint 返回常见发行版的 fuse3 安装命令。
func fuse3InstallHint() string {
	if _, err := exec.LookPath("apt"); err == nil {
		return "  sudo apt install fuse3"
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return "  sudo dnf install fuse3"
	}
	if _, err := exec.LookPath("pacman"); err == nil {
		return "  sudo pacman -S fuse3"
	}
	return "  请用发行版的包管理器安装 fuse3（如 apt/dnf/pacman install fuse3）"
}
