//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

// openDirectory 在资源管理器中打开挂载目录；target 可以是盘符（"Z:"）或目录。
func openDirectory(target string) error {
	path := strings.TrimPrefix(target, `\\?\`)
	if len(path) == 2 && path[1] == ':' {
		path += `\`
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("挂载点不可用: %s", target)
	}

	// 交给外壳打开（等价于在资源管理器里双击该目录）；目录已经打开过时会切到已有窗口
	if err := w32.ShellExecute(0, "open", path, "", "", w32.SW_SHOWNORMAL); err == nil {
		return nil
	}

	// 外壳调用失败时退回直接启动资源管理器
	cmd := exec.Command("explorer.exe", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开文件夹失败: %w", err)
	}
	// explorer 的退出码不表示打开是否成功，异步回收进程即可
	go func() { _ = cmd.Wait() }()
	return nil
}
