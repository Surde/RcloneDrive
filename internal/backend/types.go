package backend

// FieldType 是 RemoteService 表单字段的类型标识。
type FieldType string

const (
	TypeString   FieldType = "string"
	TypeInt      FieldType = "int"
	TypeBool     FieldType = "bool"
	TypePassword FieldType = "password"
	TypeSelect   FieldType = "select"
)

// FieldOption 是 select 类型字段的候选项。
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FieldDef 描述表单中的一个字段，同时是生成 rclone 配置项的 key 来源。
type FieldDef struct {
	Key       string        `json:"key"`
	Label     string        `json:"label"`
	Type      FieldType     `json:"type"`
	Required  bool          `json:"required"`
	Sensitive bool          `json:"sensitive"`
	Advanced  bool          `json:"advanced"`
	Default   string        `json:"default"`
	Options   []FieldOption `json:"options,omitempty"`
	Help      string        `json:"help"`
}

// BackendDef 描述一种远程存储协议，是前端表单与 rclone 配置生成的唯一事实来源。
type BackendDef struct {
	Type   string     `json:"type"`
	Name   string     `json:"name"`
	Label  string     `json:"label"`
	Fields []FieldDef `json:"fields"`
}
