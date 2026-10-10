package backend

// SFTP 返回 SFTP 协议的字段定义。
func SFTP() BackendDef {
	return BackendDef{
		Type:  "sftp",
		Name:  "sftp",
		Label: "SFTP",
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
				Default: "22",
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
				Help:      "与私钥二选一",
			},
			{
				Key:      "key_file",
				Label:    "私钥文件",
				Type:     TypeString,
				Advanced: true,
				Help:     "私钥路径，与密码二选一",
			},
			{
				Key:       "key_file_pass",
				Label:     "私钥密码",
				Type:      TypePassword,
				Sensitive: true,
				Advanced:  true,
			},
		},
	}
}
