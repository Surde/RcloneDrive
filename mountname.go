package main

import (
	"strings"
	"unicode"
)

// mountNameDir 把驱动器名转成安全的目录名：只保留字母、数字、空格和 - _ .，
// 其余字符（含路径分隔符）替换为 _，防止路径穿越或建目录失败；
// 过滤后为空时退化为 "drive"。跨平台共享（Windows 不使用，但编译需要）。
func mountNameDir(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		return s
	}
	return "drive"
}
