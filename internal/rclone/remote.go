package rclone

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rclone/rclone/fs"
)

const testTimeout = 20 * time.Second

// TestRemote 在进程内建立连接并列出根目录，验证远程存储是否可用。
func TestRemote(name string) error {
	if err := Ensure(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	f, err := fs.NewFs(ctx, name+":")
	if err != nil {
		return fmt.Errorf("连接测试失败: %w", err)
	}
	if _, err := f.List(ctx, ""); err != nil && !errors.Is(err, fs.ErrorDirNotFound) {
		return fmt.Errorf("连接测试失败: %w", err)
	}
	return nil
}
