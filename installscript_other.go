//go:build !windows

package main

import "errors"

// installAfterExit 非 Windows 平台不会走到这里：isInstalledBuild 恒为 false，
// 安装版安装流程只有 Windows 的 NSIS 分发会用。兜底报错防止误调用。
func installAfterExit(string) error {
	return errors.New("当前平台不支持安装包更新")
}
