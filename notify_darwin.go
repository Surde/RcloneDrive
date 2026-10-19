//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

// notifyNative 通过 macOS 通知中心显示通知（osascript）。
func notifyNative(title, body string) {
	script := "display notification " + appleScriptString(body) + " with title " + appleScriptString(title)
	_ = exec.Command("osascript", "-e", script).Start()
}

// appleScriptString 把文本转成 AppleScript 字符串字面量。
func appleScriptString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n', '\r':
			b.WriteString("  ")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
