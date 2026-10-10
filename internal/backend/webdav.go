package backend

// WebDAV 返回 WebDAV 协议的字段定义。
func WebDAV() BackendDef {
	return BackendDef{
		Type:  "webdav",
		Name:  "webdav",
		Label: "WebDAV",
		Fields: []FieldDef{
			{
				Key:      "url",
				Label:    "服务器地址",
				Type:     TypeString,
				Required: true,
			},
			{
				Key:     "vendor",
				Label:   "服务商",
				Type:    TypeSelect,
				Default: "other",
				Options: []FieldOption{
					{Value: "other", Label: "其他"},
					{Value: "nextcloud", Label: "Nextcloud"},
					{Value: "owncloud", Label: "ownCloud"},
					{Value: "sharepoint", Label: "SharePoint"},
				},
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
		},
	}
}
