//go:build linux

package main

import (
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// systemProxy 读取 Linux 系统代理：优先 HTTPS_PROXY / HTTP_PROXY 环境变量，
// 否则查 GNOME 的系统代理（gsettings org.gnome.system.proxy，与「设置 → 网络 →
// 网络代理」一致）。KDE 等未覆盖的桌面回落到环境变量。返回 nil 表示无系统代理。
func systemProxy() *url.URL {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			if u, err := url.Parse(v); err == nil && u.Host != "" {
				return u
			}
		}
	}
	if gsettingsGet("org.gnome.system.proxy", "mode") != "'manual'" {
		return nil
	}
	if host, port := gsettingsGet("org.gnome.system.proxy.https", "host"), gsettingsGet("org.gnome.system.proxy.https", "port"); host != "''" {
		if u := mustProxyURL("http", strings.Trim(host, "'"), strings.Trim(port, "'")); u != nil {
			return u
		}
	}
	if host, port := gsettingsGet("org.gnome.system.proxy.http", "host"), gsettingsGet("org.gnome.system.proxy.http", "port"); host != "''" {
		if u := mustProxyURL("http", strings.Trim(host, "'"), strings.Trim(port, "'")); u != nil {
			return u
		}
	}
	return nil
}

// gsettingsGet 读取 GNOME 设置项；gsettings 不存在或读取失败返回空串。
func gsettingsGet(schema, key string) string {
	out, err := exec.Command("gsettings", "get", schema, key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// localProxyFallback 非 Windows 平台没有本机代理兜底，返回 nil。
func localProxyFallback() *url.URL { return nil }
