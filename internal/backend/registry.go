package backend

// All 返回所有已支持的协议定义。
func All() []BackendDef {
	return []BackendDef{
		WebDAV(),
		SFTP(),
		FTP(),
	}
}

// Get 按类型名查找协议定义。
func Get(typ string) (BackendDef, bool) {
	for _, def := range All() {
		if def.Type == typ {
			return def, true
		}
	}
	return BackendDef{}, false
}
