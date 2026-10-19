package main

import "github.com/egoist/mygo"

// appVersion 是当前版本的兜底值（原始 go build/go run，无 mygo 注入时）。
// 版本号以 mygo.json 的 version 为准：`mygo dev`/`mygo build` 会把它注入
// 运行时（见 currentAppVersion）；`task build` 的裸 exe 则由
// -ldflags "-X main.appVersion=x.y.z" 注入这个变量。
var appVersion = "0.2.0"

// currentAppVersion 返回生效的应用版本：优先取 mygo 打包元数据
// （mygo dev/build 从 mygo.json 注入），取不到时用 appVersion 兜底。
func currentAppVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return appVersion
}
