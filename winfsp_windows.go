//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "embed"

	"golang.org/x/sys/windows/registry"
)

// winfsp-2.1.25156.msi 内嵌的 WinFsp 官方安装包（GPLv3，许可全文见
// third_party/winfsp/winfsp.LICENSE.txt）。放在 third_party/ 而非
// resources/：mygo 打包只拷贝 resources/，WinFsp 仅 Windows 需要，
// 避免 linux/darwin 的包携带这几 MB 安装包。首次挂载检测到系统未
// 安装 WinFsp 时提取并提权静默安装（方案：免独立下载安装器）。
//
//go:embed third_party/winfsp/winfsp-2.1.25156.msi
var winfspMSI []byte

const winfspMSIName = "winfsp-2.1.25156.msi"

// winfspInstallMu 串行化安装过程，避免并发挂载触发多次 UAC。
var winfspInstallMu sync.Mutex

// winfspInstalled 报告 WinFsp 驱动服务是否已注册。
// WinFsp 2.x 的驱动服务名带构建时间戳后缀（如 WinFsp+20261019T075328Z），
// 不能查固定名 WinFsp；这里枚举服务项按前缀匹配（同时兼容 1.x 的 WinFsp）。
func winfspInstalled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services`, registry.READ)
	if err != nil {
		return false
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return false
	}
	for _, name := range names {
		if strings.HasPrefix(name, "WinFsp") {
			return true
		}
	}
	return false
}

// appInstalledBySetup 报告应用是否由 NSIS 安装器安装（HKLM 卸载注册项存在）。
// 安装版在运行时不再尝试内嵌安装 WinFsp：组件缺失说明安装被破坏，
// 提示重装；便携版（无卸载注册项）才走内嵌静默安装兜底。
func appInstalledBySetup() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive`, registry.READ)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

// ensureWinFsp 挂载前确保 WinFsp 可用：已安装则直接返回；未安装时
// 便携版提示用户后提取内嵌 MSI，经 UAC 提权静默安装并等待注册完成；
// 安装版不自动装，返回错误提示修复安装。用户取消 UAC 或安装失败时
// 返回错误，挂载中止。
func ensureWinFsp() error {
	if winfspInstalled() {
		return nil
	}
	winfspInstallMu.Lock()
	defer winfspInstallMu.Unlock()
	if winfspInstalled() {
		return nil
	}

	if appInstalledBySetup() {
		return fmt.Errorf("缺少 WinFsp 文件系统组件（安装时未完成或被卸载），" +
			"请重新运行安装程序修复后再挂载")
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
