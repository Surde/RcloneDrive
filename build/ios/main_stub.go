//go:build !ios

package main

// main 仅供桌面平台 `go build ./...` 校验通过；
// iOS 构建由 build/ios 下的 Wails 脚手架接管。
func main() {}
