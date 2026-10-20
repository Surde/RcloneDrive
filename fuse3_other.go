//go:build !linux

package main

// ensureFuse3 非 Linux 平台不需要 FUSE3。
func ensureFuse3() error { return nil }
