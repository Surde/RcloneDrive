//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	_ "embed"

	"golang.org/x/sys/windows/registry"
)

// winfsp-2.1.25156.msi 内嵌的 WinFsp 官方安装包（GPLv3，许可全文见
// resources/winfsp.LICENSE.txt，随打包资源一并发行）。首次挂载检测到
// 系统未安装 WinFsp 时提取并提权静默安装（方案：免独立下载安装器）。
//
//go:embed resources/winfsp-2.1.25156.msi
var winfspMSI []byte

const winfspMSIName = "winfsp-2.1.25156.msi"

// winfspInstallMu 串行化安装过程，避免并发挂载触发多次 UAC。
var winfspInstallMu sync.Mutex

// winfspInstalled 报告 WinFsp 驱动服务是否已注册（安装即注册服务，
// 驱动按需启动，无需等待服务进入运行状态）。
func winfspInstalled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\WinFsp`, registry.READ)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

// ensureWinFsp 挂载前确保 WinFsp 可用：已安装则直接返回；未安装时
// 提示用户后提取内嵌 MSI，经 UAC 提权静默安装并等待注册完成。
// 用户取消 UAC 或安装失败时返回错误，挂载中止。
func ensureWinFsp() error {
	if winfspInstalled() {
		return nil
	}
	winfspInstallMu.Lock()
	defer winfspInstallMu.Unlock()
	if winfspInstalled() {
		return nil
	}

	notifyUserTo("main", "info", "需要安装 WinFsp",
		"首次挂载云盘需要安装 WinFsp 文件系统组件，请在接下来的系统提示（UAC）中确认。")
	log.Printf("未检测到 WinFsp，开始静默安装（内嵌 %s）", winfspMSIName)

	// 提取 MSI 到临时目录（安装完成后删除；msiexec 需要真实文件路径）
	msiPath := filepath.Join(os.TempDir(), winfspMSIName)
	if err := os.WriteFile(msiPath, winfspMSI, 0o600); err != nil {
		return fmt.Errorf("释放 WinFsp 安装包失败: %w", err)
	}
	defer os.Remove(msiPath)

	// 提权静默安装：Start-Process -Verb RunAs 触发 UAC，-Wait 等安装结束。
	// 用户点「否」时 PowerShell 以非零码退出（操作被取消）。
	ps := fmt.Sprintf(
		"Start-Process msiexec.exe -ArgumentList '/i','\"%s\"','/qn','/norestart' -Verb RunAs -Wait -WindowStyle Hidden",
		msiPath)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("WinFsp 安装失败或被用户取消: %w", err)
	}

	// msiexec /qn 返回时安装已落盘；轮询等待驱动服务项注册出现。
	deadline := time.Now().Add(30 * time.Second)
	for !winfspInstalled() {
		if time.Now().After(deadline) {
			return fmt.Errorf("WinFsp 安装后仍未检测到驱动服务，请重启应用后重试")
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("WinFsp 安装完成")
	notifyUserTo("main", "success", "WinFsp 安装完成", "继续挂载云盘…")
	return nil
}
