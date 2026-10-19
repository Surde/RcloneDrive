//go:build !windows

package main

// isInstalledBuild 非 Windows 平台都按便携/包管理器分发处理，
// 维持 Wails updater 的二进制替换流程（linux 包由包管理器负责更新）。
func isInstalledBuild() bool { return false }
