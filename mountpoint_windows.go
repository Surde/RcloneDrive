//go:build windows

package main

import "golang.org/x/sys/windows"

// nextFreeMountPoint 返回从 Z 往下第一个未被占用的盘符（如 "Z:"）。
func nextFreeMountPoint() string {
	drives, err := windows.GetLogicalDrives()
	if err != nil {
		return "Z:"
	}
	for i := int('Z'); i >= int('C'); i-- {
		if drives&(1<<(i-int('A'))) == 0 {
			return string(rune(i)) + ":"
		}
	}
	return ""
}

// usedDriveLetters 返回当前已被占用的盘符（如 "C:"、"Y:"），供界面过滤下拉选项。
func usedDriveLetters() []string {
	drives, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	letters := make([]string, 0, 26)
	for i := int('A'); i <= int('Z'); i++ {
		if drives&(1<<(i-int('A'))) != 0 {
			letters = append(letters, string(rune(i))+":")
		}
	}
	return letters
}
