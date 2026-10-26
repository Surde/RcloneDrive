package rclone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rclone/rclone/fs"
	fslog "github.com/rclone/rclone/fs/log"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/config/obscure"
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

// 日志轮转参数：单文件 5MiB、保留 3 份并压缩，磁盘占用有界（约 20MiB）。
const (
	logMaxSizeBytes = 5 * 1024 * 1024
	logMaxBackups   = 3
)

// LogsDir 返回日志目录（<ConfigDir>/logs），不存在时创建。
func LogsDir() (string, error) {
	return logsDirAt(ConfigDir())
}

func logsDirAt(configRoot string) (string, error) {
	dir := filepath.Join(configRoot, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建日志目录失败: %w", err)
	}
	return dir, nil
}

// SetupLogging 把应用与内嵌 rclone 的日志统一写入 <ConfigDir>/logs/rclonedrive.log
// （rclone 内置 lumberjack 轮转）。InitLogging 会 slog.SetDefault，标准库 log 的
// 输出同样进该文件——生产构建 -H=windowsgui 无控制台，这是唯一的日志出口。
// 应在 main 最早期调用，之后的日志才不丢。
func SetupLogging() error {
	return setupLoggingAt(ConfigDir())
}

func setupLoggingAt(configRoot string) error {
	dir, err := logsDirAt(configRoot)
	if err != nil {
		return err
	}
	fslog.Opt.File = filepath.Join(dir, "rclonedrive.log")
	// MaxSize 单位是字节（InitLogging 内部换算为 MiB）；<=0 表示追加不轮转
	fslog.Opt.MaxSize = fs.SizeSuffix(logMaxSizeBytes)
	fslog.Opt.MaxBackups = logMaxBackups
	fslog.Opt.Compress = true
	fslog.InitLogging()
	return nil
}

var (
	ensureOnce sync.Once
	ensureErr  error
)

// Ensure 让内嵌的 rclone 使用本应用的配置文件，只初始化一次。
func Ensure() error {
	ensureOnce.Do(func() {
		if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
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

// ReadRemote 从配置文件读取某个远程存储的全部选项，密码字段由 rclone 混淆态解密回明文。
// remotes.json 不再保存 Options（避免凭证明文落盘），编辑远程时用它来回填表单。
// 与 rclone 写配置的逻辑一致：Reveal 失败说明存的是明文，原样返回。
func ReadRemote(name string) (map[string]string, error) {
	if err := Ensure(); err != nil {
		return nil, err
	}
	data := config.LoadedData()
	if !data.HasSection(name) {
		return nil, fmt.Errorf("远程存储不存在: %s", name)
	}
	passwordKeys := map[string]bool{}
	if typ, _ := data.GetValue(name, "type"); typ != "" {
		if fields, err := Fields(typ); err == nil {
			for _, field := range fields {
				if field.Password {
					passwordKeys[field.Key] = true
				}
			}
		}
	}
	options := map[string]string{}
	for _, key := range data.GetKeyList(name) {
		if key == "type" {
			continue
		}
		value, _ := data.GetValue(name, key)
		if passwordKeys[key] {
			if revealed, err := obscure.Reveal(value); err == nil {
				value = revealed
			}
		}
		options[key] = value
	}
	return options, nil
}

// DeleteRemote 从配置文件中移除一个远程。
func DeleteRemote(name string) error {
	if err := Ensure(); err != nil {
		return err
	}
	config.DeleteRemote(name)
	return nil
}
