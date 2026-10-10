//go:build !windows && !darwin && !linux

package main

import "errors"

// openDirectory 在不支持的平台上直接报错。
func openDirectory(string) error {
	return errors.New("当前平台不支持打开文件夹")
}
