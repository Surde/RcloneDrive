//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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

	// 直接启动资源管理器（等价于在资源管理器里双击该目录）
	cmd := exec.Command("explorer.exe", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开文件夹失败: %w", err)
	}
	// explorer 的退出码不表示打开是否成功，异步回收进程即可
	go func() { _ = cmd.Wait() }()
	return nil
}
