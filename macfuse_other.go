//go:build !darwin

package main

// ensureMacFUSE 非 macOS 平台不需要 macFUSE。
func ensureMacFUSE() error { return nil }
