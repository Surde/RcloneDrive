package rclone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/rc"
)

// ConfigDir 返回应用配置目录（<UserConfigDir>/RcloneDrive）。
func ConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
	}
	return filepath.Join(dir, "RcloneDrive")
}

// ConfigPath 返回 rclone.conf 的完整路径。
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "rclone.conf")
}

var (
	ensureOnce sync.Once
	ensureErr  error
)

// Ensure 让内嵌的 rclone 使用本应用的配置文件，只初始化一次。
func Ensure() error {
	ensureOnce.Do(func() {
		if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
			ensureErr = fmt.Errorf("创建配置目录失败: %w", err)
			return
		}
		if err := config.SetConfigPath(ConfigPath()); err != nil {
			ensureErr = fmt.Errorf("设置 rclone 配置路径失败: %w", err)
			return
		}
		configfile.Install()
	})
	return ensureErr
}

// Names 返回配置文件中已保存的远程名称。
func Names() ([]string, error) {
	if err := Ensure(); err != nil {
		return nil, err
	}
	return config.GetRemoteNames(), nil
}

// Exists 报告指定远程是否已写入配置文件。
func Exists(name string) (bool, error) {
	names, err := Names()
	if err != nil {
		return false, err
	}
	for _, remote := range names {
		if remote == name {
			return true, nil
		}
	}
	return false, nil
}

// SaveRemote 写入（或覆盖）一个远程配置；密码类字段由 rclone 按后端定义自动混淆。
func SaveRemote(name, typ string, options map[string]string) error {
	if err := Ensure(); err != nil {
		return err
	}
	keyValues := rc.Params{}
	for key, value := range options {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.ContainsAny(key, "\n\r") || strings.ContainsAny(value, "\n\r") {
			return fmt.Errorf("字段 %s 不能包含换行符", key)
		}
		keyValues[key] = value
	}
	_, err := config.CreateRemote(context.Background(), name, typ, keyValues, config.UpdateRemoteOpt{
		Obscure:        true,
		NoOutput:       true,
		NonInteractive: true,
	})
	if err != nil {
		return fmt.Errorf("保存 rclone 配置失败: %w", err)
	}
	return nil
}

// DeleteRemote 从配置文件中移除一个远程。
func DeleteRemote(name string) error {
	if err := Ensure(); err != nil {
		return err
	}
	config.DeleteRemote(name)
	return nil
}
