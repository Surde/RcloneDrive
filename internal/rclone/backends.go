package rclone

// 注册 rclone 的存储后端；挂载实现（cmount）通过 init 注册，
// 未启用 cmount 构建标签时注册为空，挂载能力由 MountAvailable 报告。
//
// 全量引入 backend/all（68 个后端），界面上的连接类型目录（见 catalog.go）就是
// 从这里注册进 fs.Registry 的后端；代价是二进制体积明显变大。
import (
	_ "github.com/rclone/rclone/backend/all"
	// cmd/cmount 提供挂载实现（Windows 走 WinFsp）
	_ "github.com/rclone/rclone/cmd/cmount"
)
