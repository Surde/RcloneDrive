//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// openDirectory 在访达中打开挂载目录。
func openDirectory(target string) error {
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("挂载点不可用: %s", target)
	}
	cmd := exec.Command("open", target)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开文件夹失败: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
