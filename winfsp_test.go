//go:build windows

package main

import "testing"

// 内嵌的 WinFsp 安装包完整性冒烟：官方 MSI 约 2MB+，显著小于此值
// 说明 embed 路径或文件损坏。
func TestWinfspEmbeddedMSI(t *testing.T) {
	if len(winfspMSI) < 1<<20 {
		t.Fatalf("内嵌 WinFsp MSI 异常小: %d 字节", len(winfspMSI))
	}
}

// winfspInstalled 在本机（未安装 WinFsp 的开发机）应返回 false；
// 已安装环境返回 true 也合法，这里只验证调用不 panic。
func TestWinfspInstalled(t *testing.T) {
	t.Logf("winfspInstalled() = %v", winfspInstalled())
}
