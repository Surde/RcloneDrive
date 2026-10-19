//go:build windows

package main

import (
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxy 读取 Windows 系统代理（IE/WinINET 设置，浏览器与系统下载都走这里）。
// 返回 nil 表示未启用，调用方回落到 HTTPS_PROXY 环境变量。
func systemProxy() *url.URL {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()

	enable, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enable == 0 {
		return nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(server) == "" {
		return nil
	}

	// ProxyServer 两种形式：整体 "127.0.0.1:7897"，或分协议 "http=..:..;https=..:..;socks=..:.."
	var hostport string
	if strings.Contains(server, "=") {
		for _, part := range strings.Split(server, ";") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) != 2 {
				continue
			}
			if strings.EqualFold(kv[0], "https") {
				hostport = kv[1]
				break
			}
			if hostport == "" && strings.EqualFold(kv[0], "http") {
				hostport = kv[1]
			}
		}
	} else {
		hostport = strings.TrimSpace(server)
	}
	if hostport == "" {
		return nil
	}
	if !strings.Contains(hostport, "://") {
		hostport = "http://" + hostport
	}
	u, err := url.Parse(hostport)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}
