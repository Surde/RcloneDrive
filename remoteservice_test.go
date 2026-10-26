package main

import (
	"path/filepath"
	"testing"

	"rclonedrive/internal/rclone"

	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
)

// useTestEnv 把 rclone 配置与本服务的元数据都指到临时目录，隔离真实用户配置。
// 写函数已不做防御性 Ensure（见 internal/rclone.Ensure 注释），这里自行
// SetConfigPath+Install 即等价于 Ensure 的核心步骤。
func useTestEnv(t *testing.T) *RemoteService {
	t.Helper()
	dir := t.TempDir()
	prev := config.GetConfigPath()
	t.Cleanup(func() {
		_ = config.SetConfigPath(prev)
		configfile.Install()
	})
	if err := config.SetConfigPath(filepath.Join(dir, "rclone.conf")); err != nil {
		t.Fatalf("SetConfigPath: %v", err)
	}
	configfile.Install()
	svc := &RemoteService{
		metaPath: filepath.Join(dir, "remotes.json"),
		mounts:   rclone.NewMountManager(),
	}
	return svc
}

func webdavRemote(name, url string) Remote {
	return Remote{
		Name: name, Type: "webdav", AutoConnect: true,
		Options: map[string]string{"url": url, "vendor": "other", "pass": "secret"},
	}
}

// Task 2.1：改名成功——rclone 配置节迁移、元数据条目更新。
func TestUpdateRenames(t *testing.T) {
	svc := useTestEnv(t)
	if err := svc.Create(webdavRemote("A", "https://a.example")); err != nil {
		t.Fatalf("Create A: %v", err)
	}

	renamed := webdavRemote("B", "https://a.example")
	if err := svc.Update("A", renamed); err != nil {
		t.Fatalf("Update rename: %v", err)
	}

	if _, ok := svc.Get("B"); !ok {
		t.Fatal("元数据应有 B")
	}
	if _, ok := svc.Get("A"); ok {
		t.Fatal("元数据不应再有 A")
	}
	data := config.LoadedData()
	if !data.HasSection("B") || data.HasSection("A") {
		t.Fatalf("rclone 配置应有 B 无 A: B=%v A=%v", data.HasSection("B"), data.HasSection("A"))
	}
	// 重启视角：从盘上重读元数据，B 仍在（remotes.json 已落盘）
	svc2 := &RemoteService{metaPath: svc.metaPath, mounts: svc.mounts}
	if err := svc2.load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := svc2.Get("B"); !ok {
		t.Fatal("重读后元数据应有 B")
	}
}

// Task 2.1：新名占用拒绝，两个驱动器均保持不变。
func TestUpdateRenameConflict(t *testing.T) {
	svc := useTestEnv(t)
	if err := svc.Create(webdavRemote("A", "https://a")); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if err := svc.Create(webdavRemote("C", "https://c")); err != nil {
		t.Fatalf("Create C: %v", err)
	}

	if err := svc.Update("A", webdavRemote("C", "https://a")); err == nil {
		t.Fatal("新名占用应报错")
	}
	if _, ok := svc.Get("A"); !ok {
		t.Error("A 应保持不变")
	}
	if data := config.LoadedData(); !data.HasSection("A") {
		t.Error("A 的配置节应保持不变")
	}
}

// Task 2.1：oldName 不存在拒绝。
func TestUpdateOldNameMissing(t *testing.T) {
	svc := useTestEnv(t)
	if err := svc.Update("X", webdavRemote("B", "https://b")); err == nil {
		t.Fatal("oldName 不存在应报错")
	}
}

// Task 2.1：名称未变化走既有路径（配置节名不变、选项更新）。
func TestUpdateSameName(t *testing.T) {
	svc := useTestEnv(t)
	if err := svc.Create(webdavRemote("A", "https://old")); err != nil {
		t.Fatalf("Create A: %v", err)
	}

	if err := svc.Update("A", webdavRemote("A", "https://new")); err != nil {
		t.Fatalf("Update same name: %v", err)
	}
	if data := config.LoadedData(); !data.HasSection("A") {
		t.Fatal("配置节应仍为 A")
	}
	opts, err := rclone.ReadRemote("A")
	if err != nil {
		t.Fatalf("ReadRemote: %v", err)
	}
	if opts["url"] != "https://new" {
		t.Errorf("url = %q, want https://new", opts["url"])
	}
}
