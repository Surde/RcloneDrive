//go:build windows

package main

import (
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

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

var localProxyCache struct {
	sync.Mutex
	u      *url.URL
	probed time.Time
}

const localProxyCacheTTL = 30 * time.Second

// localProxyFallback 系统代理未启用时的本机代理兜底：注册表里残留的
// ProxyServer（如 Clash 的 127.0.0.1:7897）在本机端口可连通时作为代理
// 使用。覆盖「系统代理开关没开 / TUN 未接管、但本机代理客户端在跑」的
// 常见场景（浏览器走插件代理不受影响，而应用直连被网络阻断）。
// 只探测回环地址，结果缓存 30 秒；代理不在时回环端口连接被拒是瞬时的。
func localProxyFallback() *url.URL {
	localProxyCache.Lock()
	defer localProxyCache.Unlock()
	if time.Since(localProxyCache.probed) < localProxyCacheTTL {
		return localProxyCache.u
	}
	localProxyCache.probed = time.Now()
	localProxyCache.u = probeLocalProxy()
	return localProxyCache.u
}

func probeLocalProxy() *url.URL {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil
	}
	// 与 systemProxy 相同的解析，但只看整体 host:port 形式（含 = 的分协议
	// 形式只在系统代理启用时有意义）
	hostport := strings.TrimSpace(server)
	if hostport == "" || strings.Contains(hostport, "=") {
		return nil
	}
	if strings.Contains(hostport, "://") {
		u, err := url.Parse(hostport)
		if err != nil || u.Host == "" {
			return nil
		}
		hostport = u.Host
	}
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || !isLoopback(host) {
		return nil
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 300*time.Millisecond)
	if err != nil {
		return nil
	}
	conn.Close()
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}
}

func isLoopback(host string) bool {
	h := strings.Trim(strings.ToLower(host), "[]")
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}
