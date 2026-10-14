//go:build !windows && !darwin && !linux

package main

import "net/url"

// systemProxy 其他平台不读系统代理，返回 nil 让 Transport 回落到
// HTTPS_PROXY 环境变量（http.DefaultTransport 的默认行为）。
func systemProxy() *url.URL { return nil }
