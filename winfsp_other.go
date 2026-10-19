//go:build !windows

package main

// ensureWinFsp 在非 Windows 平台是空操作（FUSE/macFUSE 由用户自行安装）。
func ensureWinFsp() error { return nil }
