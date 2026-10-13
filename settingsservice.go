package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"rclone-drive/internal/rclone"
)

// AutoStartStatus 描述开机自启的当前状态。
type AutoStartStatus struct {
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
	Target    string `json:"target"`
}

// appSettings 是 settings.json 的内容，新增设置项时在这里加字段。
type appSettings struct {
	// AutoUpdate 为 true 时应用启动后自动检查更新并按周期复查；
	// 默认 false，自动更新完全由用户显式开启。
	AutoUpdate bool `json:"autoUpdate"`
}

// SettingsService 提供应用级设置的读写。
// 设置持久化在 <ConfigDir>/settings.json（0600）。
type SettingsService struct {
	mu      sync.Mutex
	settings appSettings
	loaded  bool
}

func (s *SettingsService) path() string {
	return filepath.Join(rclone.ConfigDir(), "settings.json")
}

func (s *SettingsService) load() appSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.settings
	}
	s.loaded = true
	data, err := os.ReadFile(s.path())
	if err != nil {
		return s.settings
	}
	var settings appSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return s.settings
	}
	s.settings = settings
	return s.settings
}

func (s *SettingsService) save(settings appSettings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path(), data, 0o600)
}

// AutoStart 返回当前的开机自启状态。
func (s *SettingsService) AutoStart() (AutoStartStatus, error) {
	if !autoStartSupported() {
		return AutoStartStatus{}, nil
	}
	enabled, target, err := readAutoStart()
	if err != nil {
		return AutoStartStatus{}, err
	}
	return AutoStartStatus{Enabled: enabled, Supported: true, Target: target}, nil
}

// SetAutoStart 打开或关闭开机自启，自启项指向当前可执行文件。
func (s *SettingsService) SetAutoStart(enabled bool) error {
	if !autoStartSupported() {
		return errors.New("当前平台不支持开机自启")
	}
	target, err := autoStartTarget()
	if err != nil {
		return err
	}
	return writeAutoStart(target, enabled)
}

// AutoUpdate 返回用户是否开启了自动检查更新（默认关闭）。
func (s *SettingsService) AutoUpdate() bool {
	return s.load().AutoUpdate
}

// SetAutoUpdate 打开或关闭自动检查更新，立即持久化。
func (s *SettingsService) SetAutoUpdate(enabled bool) error {
	s.mu.Lock()
	settings := s.settings
	settings.AutoUpdate = enabled
	s.settings = settings
	s.loaded = true
	s.mu.Unlock()
	return s.save(settings)
}
