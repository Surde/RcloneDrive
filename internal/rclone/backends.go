package rclone

// 注册 rclone 的存储后端；挂载实现（cmount）通过 init 注册，
// 未启用 cmount 构建标签时注册为空，挂载能力由 MountAvailable 报告。
import (
	_ "github.com/rclone/rclone/backend/ftp"
	// local 后端：既是可挂载的本地存储，也是 --vfs-cache-mode 写缓存的数据缓存后端
	_ "github.com/rclone/rclone/backend/local"
	_ "github.com/rclone/rclone/backend/sftp"
	_ "github.com/rclone/rclone/backend/webdav"
	_ "github.com/rclone/rclone/cmd/cmount"
)
