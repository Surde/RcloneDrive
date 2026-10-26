package main

import (
	"testing"

	"rclonedrive/internal/rclone"

	"github.com/egoist/mygo/ui"
)

// 视图冒烟测试：ui.Tester 在无窗口环境下渲染视图、读文本、模拟点击。
// 服务使用真实实现（RemoteService 读测试机的 remotes.json，通常为空列表；
// 不写配置、不做网络操作），覆盖三个窗口的关键渲染路径。

func testServices() {
	mountManager = rclone.NewMountManager()
	remoteService = NewRemoteService(mountManager)
	mountService = NewMountService(remoteService, mountManager)
	systemService = &SystemService{}
	settingsService = &SettingsService{}
	updateService = NewUpdateService(settingsService)
}

// TestMainView 主窗口：标题栏、空状态、添加驱动器按钮。
func TestMainView(t *testing.T) {
	testServices()
	state := &mainViewState{}
	tt := ui.NewTester(state.view, 580, 540)
	if !tt.HasText("RcloneDrive") {
		t.Fatalf("texts %q do not contain the app title", tt.Texts())
	}
	if !tt.HasText("还没有虚拟驱动器") {
		t.Fatalf("texts %q do not contain the empty state", tt.Texts())
	}
	if err := tt.Click("添加驱动器"); err != nil {
		// allWindows 为 nil 时点击是空操作，不应出错
		t.Fatalf("click 添加驱动器: %v", err)
	}
}

// TestDriveEditorView 驱动器编辑窗口：标题、类型树、表单与底部按钮。
func TestDriveEditorView(t *testing.T) {
	testServices()
	state := newDriveEditorState()
	tt := ui.NewTester(state.view, driveWindowW, driveWindowH)

	if !tt.HasText("新虚拟驱动器") {
		t.Fatalf("texts %q do not contain the window title", tt.Texts())
	}
	// 左侧类型树默认展开「常用协议」分组（第一个）
	if !tt.HasText("常用协议") {
		t.Fatalf("texts %q do not contain the first catalog group", tt.Texts())
	}
	// 右侧表单：名称 + 连接设置 + 底部按钮
	if !tt.HasText("名称") || !tt.HasText("连接设置") {
		t.Fatalf("texts %q do not contain the form sections", tt.Texts())
	}
	for _, label := range []string{"测试连接", "取消", "保存", "保存并连接"} {
		if !tt.HasText(label) {
			t.Fatalf("texts %q do not contain footer button %q", tt.Texts(), label)
		}
	}

	// 说明页
	state.tab = 2
	tt.Frame()
	if !tt.HasText("文档") {
		t.Fatalf("texts %q do not contain the about tab", tt.Texts())
	}
}

// TestSettingsView 设置窗口：标题栏与三个分区。
func TestSettingsView(t *testing.T) {
	testServices()
	state := &settingsViewState{}
	tt := ui.NewTester(state.view, settingsWindowW, settingsWindowH)
	for _, label := range []string{"设置", "外观", "启动", "更新"} {
		if !tt.HasText(label) {
			t.Fatalf("texts %q do not contain %q", tt.Texts(), label)
		}
	}
	// 切到「更新」分区：当前版本与检查更新按钮
	if err := tt.Click("更新"); err != nil {
		t.Fatalf("click 更新: %v", err)
	}
	if !tt.HasText("当前版本") || !tt.HasText("检查更新") {
		t.Fatalf("texts %q do not contain the update section", tt.Texts())
	}

	// 「启动」分区的日志入口：只验证渲染，不点击（点击会真实打开资源管理器）
	if err := tt.Click("启动"); err != nil {
		t.Fatalf("click 启动: %v", err)
	}
	if !tt.HasText("打开日志文件夹") {
		t.Fatalf("texts %q do not contain the logs entry", tt.Texts())
	}
}

// TestFormatters 数值格式化与旧前端 lib/format.ts 对齐。
func TestFormatters(t *testing.T) {
	sizeCases := []struct {
		bytes float64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{2048, "2.0 KB"},
		{18.7 * 1024 * 1024 * 1024, "18.7 GB"},
	}
	for _, tc := range sizeCases {
		if got := formatSize(tc.bytes); got != tc.want {
			t.Errorf("formatSize(%v) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
	speedCases := []struct {
		bps  float64
		want string
	}{
		{0, "0 B/s"},
		{100, "100 B/s"},
		{1536, "1.50 KB/s"},
		{12.345 * 1024 * 1024, "12.3 MB/s"},
	}
	for _, tc := range speedCases {
		if got := formatSpeed(tc.bps); got != tc.want {
			t.Errorf("formatSpeed(%v) = %q, want %q", tc.bps, got, tc.want)
		}
	}
}

// TestProviderMatches 枚举字段的 provider 过滤（DriveWindow.vue 的 providerMatches）。
func TestProviderMatches(t *testing.T) {
	cases := []struct {
		filter, provider string
		want             bool
	}{
		{"", "anything", true},
		{"aws", "aws", true},
		{"aws", "gcp", false},
		{"aws,gcp", "gcp", true},
		{"!aws", "aws", false},
		{"!aws", "gcp", true},
		{"aws,!secret", "aws", true},
		{"aws,!aws", "aws", false},
		{"", "", true},
	}
	for _, tc := range cases {
		if got := providerMatches(tc.filter, tc.provider); got != tc.want {
			t.Errorf("providerMatches(%q, %q) = %v, want %v", tc.filter, tc.provider, got, tc.want)
		}
	}
}

// TestSegmentFields 连续 bool 字段分段（DriveWindow.vue 的 segmentFields）。
func TestSegmentFields(t *testing.T) {
	fields := []rclone.Field{
		{Key: "a", Type: "string"},
		{Key: "b", Type: "bool"},
		{Key: "c", Type: "bool"},
		{Key: "d", Type: "string"},
	}
	segs := segmentFields(fields)
	if len(segs) != 3 {
		t.Fatalf("segmentFields returned %d segments, want 3", len(segs))
	}
	if len(segs[1].bools) != 2 {
		t.Fatalf("middle segment has %d bools, want 2", len(segs[1].bools))
	}
	if segs[0].field.Key != "a" || segs[2].field.Key != "d" {
		t.Fatalf("unexpected field segments: %+v", segs)
	}
}
