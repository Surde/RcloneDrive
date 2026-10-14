package main

import "net/url"

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
