//go:build !windows && !darwin && !linux

package main

// notifyNative 在无原生通知支持的平台上为空实现。
func notifyNative(string, string) {}
