package backend

// FTP 返回 FTP 协议的字段定义。
func FTP() BackendDef {
	return BackendDef{
		Type:  "ftp",
		Name:  "ftp",
		Label: "FTP",
		Fields: []FieldDef{
			{
				Key:      "host",
				Label:    "主机地址",
				Type:     TypeString,
				Required: true,
			},
			{
				Key:     "port",
				Label:   "端口",
				Type:    TypeInt,
				Default: "21",
			},
			{
				Key:   "user",
				Label: "用户名",
				Type:  TypeString,
			},
			{
				Key:       "pass",
				Label:     "密码",
				Type:      TypePassword,
				Sensitive: true,
			},
			{
				Key:   "explicit_tls",
				Label: "使用 Explicit TLS (FTPS)",
				Type:  TypeBool,
			},
		},
	}
}
