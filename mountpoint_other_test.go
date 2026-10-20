//go:build !windows

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMountNameDir 驱动器名转目录名：保留常规字符、危险字符替换、空名退化。
func TestMountNameDir(t *testing.T) {
	cases := []struct {
		name, want string
	}{
		{"我的网盘", "我的网盘"},
		{"nas-01_backup.v2", "nas-01_backup.v2"},
		{`a/b\c:d`, "a_b_c_d"},
		{"...", "..."},
		{"   ", "drive"},
		{"", "drive"},
	}
	for _, tc := range cases {
		if got := mountNameDir(tc.name); got != tc.want {
			t.Errorf("mountNameDir(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestDefaultMountTargetPerDrive 默认挂载点在根目录下按驱动器名隔离，
// 且不会逃出挂载根目录（路径穿越防护）。
func TestDefaultMountTargetPerDrive(t *testing.T) {
	root := nextFreeMountPoint()
	a := defaultMountTarget("driveA")
	b := defaultMountTarget("driveB")
	if a == b {
		t.Fatalf("两个驱动器的默认挂载点相同: %q", a)
	}
	if filepath.Dir(a) != root || filepath.Dir(b) != root {
		t.Fatalf("默认挂载点应位于根目录 %q 下: %q, %q", root, a, b)
	}
	if !strings.HasPrefix(a, root) {
		t.Fatalf("默认挂载点 %q 逃出根目录 %q", a, root)
	}
}
