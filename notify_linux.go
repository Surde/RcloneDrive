//go:build linux

package main

import (
	"os/exec"
	"strings"
)

// notifyNative 通过桌面通知服务显示通知（notify-send / libnotify）。
func notifyNative(title, body string) {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return
	}
	body = strings.ReplaceAll(body, "\n", " ")
	_ = exec.Command("notify-send", "-a", "RcloneDrive", title, body).Start()
}
