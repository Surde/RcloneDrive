//go:build darwin

package main

import (
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

// systemProxy 读取 macOS 系统代理（scutil --proxy，与「系统设置 → 网络 → 代理」一致）。
// 优先 HTTPS 代理，其次 HTTP，最后 SOCKS。返回 nil 表示未启用，
// 调用方回落到 HTTPS_PROXY 环境变量。
func systemProxy() *url.URL {
	out, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return nil
	}
	settings := map[string]string{}
	lineRe := regexp.MustCompile(`^\s*(\w+)\s*:\s*(.+?)\s*$`)
	for _, line := range strings.Split(string(out), "\n") {
		if m := lineRe.FindStringSubmatch(line); m != nil {
			settings[m[1]] = m[2]
		}
	}
	if settings["HTTPSEnable"] == "1" {
		if u := mustProxyURL("http", settings["HTTPSProxy"], settings["HTTPSPort"]); u != nil {
			return u
		}
	}
	if settings["HTTPEnable"] == "1" {
		if u := mustProxyURL("http", settings["HTTPProxy"], settings["HTTPPort"]); u != nil {
			return u
		}
	}
	if settings["SOCKSEnable"] == "1" {
		if u := mustProxyURL("socks5", settings["SOCKSProxy"], settings["SOCKSPort"]); u != nil {
			return u
		}
	}
	return nil
}

// localProxyFallback 非 Windows 平台没有本机代理兜底，返回 nil。
func localProxyFallback() *url.URL { return nil }
