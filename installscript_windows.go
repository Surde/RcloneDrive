//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// installAfterExit 安排「当前进程退出后运行 NSIS 安装包（静默）并重启应用」。
// 安装包不能覆盖正在运行的 exe，所以由脚本等待本进程退出再执行：
// Wait-Process 等本进程结束 → 静默运行安装包（自动清理临时目录）→ 重新启动应用。
// 返回 nil 表示已就绪，调用方应立即退出应用。
func installAfterExit(setupPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位当前程序失败: %w", err)
	}
	dir, err := os.MkdirTemp("", "rclonedrive-install")
	if err != nil {
		return err
	}
	script := filepath.Join(dir, "install.ps1")
	content := fmt.Sprintf(`$ErrorActionPreference = 'SilentlyContinue'
Wait-Process -Id %d -Timeout 120
$p = Start-Process -FilePath '%s' -ArgumentList '/S' -Wait -PassThru
Start-Sleep -Seconds 2
if ($null -eq $p -or $p.ExitCode -eq 0) { Start-Process '%s' }
Remove-Item -Recurse -Force '%s'
`, os.Getpid(), setupPath, exe, filepath.Dir(setupPath))
	if err := os.WriteFile(script, []byte(content), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	// 安装包与脚本都在临时目录里，由上面脚本的载体在退出前删除整个目录
	bootstrap := fmt.Sprintf(`Wait-Process -Id %d -Timeout 120; powershell -NoProfile -ExecutionPolicy Bypass -File '%s'; Remove-Item -Recurse -Force '%s'`, os.Getpid(), script, dir)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", bootstrap)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x08000000 | 0x00000200 | 0x01000000, // detached | no window | new proc group | breakaway from job
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("启动安装助手失败: %w", err)
	}
	return nil
}
