package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"rclone-drive/internal/backend"
	"rclone-drive/internal/rclone"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var remoteNamePattern = regexp.MustCompile(`^\S+$`)

func init() {
	// 远程列表变化事件：主窗口据此刷新列表（新增/编辑窗口保存后触发）
	application.RegisterEvent[string]("remotes:changed")
}

// Remote 描述一个远程存储及其本地挂载偏好。
type Remote struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	MountPoint  string            `json:"mountPoint"`
	AutoConnect bool              `json:"autoConnect"`
	ReadOnly    bool              `json:"readOnly"`
	Options     map[string]string `json:"options"`
}

// RemoteService 管理远程存储的增删改查与连接测试。
type RemoteService struct {
	mu        sync.RWMutex
	remotes   []Remote
	metaPath  string
	mounts    *rclone.MountManager
	observers []func()
	app       *application.App
}

// NewRemoteService 创建服务，元数据存于 <ConfigDir>/remotes.json。
func NewRemoteService(mounts *rclone.MountManager) *RemoteService {
	return &RemoteService{
		metaPath: filepath.Join(rclone.ConfigDir(), "remotes.json"),
		mounts:   mounts,
	}
}

// OnChange 注册远程列表变化回调（用于刷新托盘菜单）；可注册多个，互不覆盖。
func (s *RemoteService) OnChange(fn func()) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, fn)
}

func (s *RemoteService) notifyChange() {
	s.mu.RLock()
	observers := make([]func(), len(s.observers))
	app := s.app
	s.mu.RUnlock()
	for _, fn := range observers {
		fn()
	}
	if app != nil {
		app.Event.Emit("remotes:changed", "")
	}
}

// ServiceStartup 启动时加载本地元数据。
func (s *RemoteService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.app = application.Get()
	s.mu.Unlock()
	return s.load()
}

func (s *RemoteService) load() error {
	data, err := os.ReadFile(s.metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("读取远程存储列表失败: %w", err)
	}
	var remotes []Remote
	if err := json.Unmarshal(data, &remotes); err != nil {
		return fmt.Errorf("解析远程存储列表失败: %w", err)
	}
	s.mu.Lock()
	s.remotes = remotes
	s.mu.Unlock()
	return nil
}

func (s *RemoteService) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.remotes, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.metaPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.metaPath, data, 0o644); err != nil {
		return err
	}
	// 列表变化后刷新托盘菜单
	s.notifyChange()
	return nil
}

// List 返回所有远程存储。
func (s *RemoteService) List() []Remote {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Remote, len(s.remotes))
	copy(out, s.remotes)
	return out
}

// Get 按名称查找远程存储。
func (s *RemoteService) Get(name string) (Remote, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, remote := range s.remotes {
		if remote.Name == name {
			return remote, true
		}
	}
	return Remote{}, false
}

// Backends 返回所有支持的协议定义，供前端渲染表单。
func (s *RemoteService) Backends() []backend.BackendDef {
	return backend.All()
}

// Create 校验并新建远程存储，敏感字段写入 INI 前会做 rclone 混淆。
func (s *RemoteService) Create(remote Remote) error {
	if err := s.validate(remote, true); err != nil {
		return err
	}
	if err := s.writeConfigSection(remote); err != nil {
		return err
	}
	s.mu.Lock()
	s.remotes = append(s.remotes, remote)
	s.mu.Unlock()
	return s.save()
}

// Update 校验并更新远程存储；若已挂载则先卸载再重写配置。
func (s *RemoteService) Update(remote Remote) error {
	if err := s.validate(remote, false); err != nil {
		return err
	}
	if s.mounts.IsMounted(remote.Name) {
		if err := s.mounts.Unmount(remote.Name); err != nil {
			return err
		}
	}
	if err := s.writeConfigSection(remote); err != nil {
		return err
	}
	s.mu.Lock()
	for i := range s.remotes {
		if s.remotes[i].Name == remote.Name {
			s.remotes[i] = remote
			break
		}
	}
	s.mu.Unlock()
	return s.save()
}

// Delete 删除远程存储：卸载挂载、移除配置节与本地元数据。
func (s *RemoteService) Delete(name string) error {
	if _, ok := s.Get(name); !ok {
		return fmt.Errorf("远程存储不存在: %s", name)
	}
	if s.mounts.IsMounted(name) {
		if err := s.mounts.Unmount(name); err != nil {
			return err
		}
	}
	if err := rclone.DeleteRemote(name); err != nil {
		return err
	}
	s.mu.Lock()
	for i := range s.remotes {
		if s.remotes[i].Name == name {
			s.remotes = append(s.remotes[:i], s.remotes[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return s.save()
}

// Test 在进程内建立连接并列举根目录，验证远程存储可用。
func (s *RemoteService) Test(name string) error {
	if _, ok := s.Get(name); !ok {
		return fmt.Errorf("远程存储不存在: %s", name)
	}
	return rclone.TestRemote(name)
}

func (s *RemoteService) validate(remote Remote, creating bool) error {
	name := strings.TrimSpace(remote.Name)
	if name == "" {
		return errors.New("名称不能为空")
	}
	if !remoteNamePattern.MatchString(name) {
		return fmt.Errorf("名称不能包含空白字符: %q", remote.Name)
	}
	def, ok := backend.Get(remote.Type)
	if !ok {
		return fmt.Errorf("不支持的协议类型: %s", remote.Type)
	}
	_, exists := s.Get(name)
	if creating && exists {
		return fmt.Errorf("远程存储已存在: %s", name)
	}
	if !creating && !exists {
		return fmt.Errorf("远程存储不存在: %s", name)
	}
	for _, field := range def.Fields {
		value := strings.TrimSpace(remote.Options[field.Key])
		if field.Required && value == "" {
			return fmt.Errorf("缺少必填字段: %s", field.Label)
		}
	}
	return nil
}

// writeConfigSection 将远程存储的选项写入 rclone.conf，密码类字段由 rclone 自动混淆。
func (s *RemoteService) writeConfigSection(remote Remote) error {
	if _, ok := backend.Get(remote.Type); !ok {
		return fmt.Errorf("不支持的协议类型: %s", remote.Type)
	}
	return rclone.SaveRemote(remote.Name, remote.Type, remote.Options)
}
