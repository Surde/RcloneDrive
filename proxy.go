package main

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

// mustProxyURL 把主机与端口拼成代理 URL；缺主机或端口非法时返回 nil。
func mustProxyURL(scheme, host, port string) *url.URL {
	if host == "" || port == "" {
		return nil
	}
	u, err := url.Parse(scheme + "://" + host + ":" + port)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

// proxyCache 缓存系统代理探测结果：注册表/scutil 查询不能太频繁（Linux
// 的 gsettings 还是子进程），同时让「应用启动后才打开系统代理」能在 TTL
// 内生效，不用重启应用。
var proxyCache struct {
	sync.Mutex
	u      *url.URL
	probed time.Time
}

const proxyCacheTTL = 15 * time.Second

// cachedSystemProxy 带 TTL 地查询系统代理（各平台实现见 proxy_*.go）。
func cachedSystemProxy() *url.URL {
	proxyCache.Lock()
	defer proxyCache.Unlock()
	if time.Since(proxyCache.probed) < proxyCacheTTL {
		return proxyCache.u
	}
	proxyCache.u = systemProxy()
	proxyCache.probed = time.Now()
	return proxyCache.u
}

// proxyForRequest 是更新器 Transport 的 Proxy 回调，优先级：
// 系统代理（缓存 TTL 内动态生效）→ HTTPS_PROXY 等环境变量 → 本机代理兜底
// （仅 Windows，见 proxy_windows.go 的 localProxyFallback）。
func proxyForRequest(req *http.Request) (*url.URL, error) {
	if u := cachedSystemProxy(); u != nil {
		return u, nil
	}
	if u, err := http.ProxyFromEnvironment(req); err == nil && u != nil {
		return u, nil
	}
	return localProxyFallback(), nil
}
