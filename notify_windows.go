//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"rclonedrive/internal/rclone"

	"golang.org/x/sys/windows/registry"
)

const (
	// 通知使用的 AUMID 与窗口共用 appID（见 appinfo.go），任务栏分组与通知身份保持一致
	notifyAppName = "RcloneDrive"
)

var notifyAppIDOnce sync.Once

// registerNotifyAppID 通过注册表登记 AUMID，让 toast 显示为 RcloneDrive 而不是 PowerShell。
func registerNotifyAppID() {
	notifyAppIDOnce.Do(func() {
		key, _, err := registry.CreateKey(registry.CURRENT_USER,
			`Software\Classes\AppUserModelId\`+appID, registry.SET_VALUE)
		if err != nil {
			return
		}
		defer key.Close()
		_ = key.SetStringValue("DisplayName", notifyAppName)
		if exe, err := os.Executable(); err == nil {
			_ = key.SetStringValue("IconUri", exe)
			_ = key.SetStringValue("IconBackgroundColor", "0")
		}
	})
}

// notifyNative 通过 Windows 通知中心弹出原生 toast（异步，不阻塞调用方）。
func notifyNative(title, body string) {
	registerNotifyAppID()

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", toastScript)
	cmd.Env = append(os.Environ(),
		"RCLONEDRIVE_TITLE="+escapeToastText(title),
		"RCLONEDRIVE_BODY="+escapeToastText(body),
		"RCLONEDRIVE_APPID="+appID,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		logNotifyFailure(err.Error())
		return
	}
	// 失败时记录到 <配置目录>/notify.log，便于排查（成功后删除旧日志）
	go func() {
		if err := cmd.Wait(); err != nil {
			logNotifyFailure(err.Error())
		}
	}()
}

// logNotifyFailure 记录通知发送失败原因，便于诊断。
func logNotifyFailure(detail string) {
	path := filepath.Join(rclone.ConfigDir(), "notify.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	entry := time.Now().Format(time.RFC3339) + " " + detail + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry)
}

// escapeToastText 转义会破坏 toast XML 的字符（标题/正文经环境变量传入模板）。
func escapeToastText(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
		"\n", " ",
		"\r", " ",
	)
	return replacer.Replace(s)
}

const toastScript = `$ErrorActionPreference='SilentlyContinue';` +
	`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime] > $null;` +
	`[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType=WindowsRuntime] > $null;` +
	`[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType=WindowsRuntime] > $null;` +
	`$template = '<toast><visual><binding template="ToastText02"><text id="1">' + $env:RCLONEDRIVE_TITLE + '</text><text id="2">' + $env:RCLONEDRIVE_BODY + '</text></binding></visual></toast>';` +
	`$xml = New-Object Windows.Data.Xml.Dom.XmlDocument;` +
	`$xml.LoadXml($template);` +
	`$toast = New-Object Windows.UI.Notifications.ToastNotification -ArgumentList $xml;` +
	`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($env:RCLONEDRIVE_APPID).Show($toast)`
