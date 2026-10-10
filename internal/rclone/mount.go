package rclone

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rclone/rclone/cmd/mountlib"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/vfs/vfscommon"
)

const (
	logMaxLines        = 200
	mountReadyTimeout  = 20 * time.Second
	mountReadyInterval = 200 * time.Millisecond
)

func stamp() string {
	return time.Now().Format(time.RFC3339)
}

// logBuffer 是固定行数的环形日志缓冲，记录一次挂载的过程信息。
type logBuffer struct {
	mu    sync.Mutex
	lines []string
	head  int
	count int
}

func newLogBuffer() *logBuffer {
	return &logBuffer{lines: make([]string, logMaxLines)}
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := strings.TrimRight(string(p), "\r\n")
	if text == "" {
		return len(p), nil
	}
	for _, line := range strings.Split(text, "\n") {
		b.lines[b.head] = line
		b.head = (b.head + 1) % logMaxLines
		if b.count < logMaxLines {
			b.count++
		}
	}
	return len(p), nil
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, b.count)
	for i := 0; i < b.count; i++ {
		out = append(out, b.lines[(b.head+i)%logMaxLines])
	}
	return strings.Join(out, "\n")
}

type mountEntry struct {
	point  *mountlib.MountPoint
	log    *logBuffer
	target string
}

// MountManager 基于内嵌的 rclone mountlib 管理挂载的生命周期。
type MountManager struct {
	mu        sync.Mutex
	mounts    map[string]*mountEntry
	observers []func(name string, mounted bool, errMsg string)
}

// NewMountManager 创建挂载管理器。
func NewMountManager() *MountManager {
	return &MountManager{mounts: make(map[string]*mountEntry)}
}

// MountType 返回当前构建可用的挂载实现（如 cmount）；无可用实现时返回空字符串。
func MountType() string {
	typ, fn := mountlib.ResolveMountMethod("")
	if fn == nil {
		return ""
	}
	return typ
}

// MountAvailable 报告当前构建是否具备挂载能力。
func MountAvailable() bool {
	return MountType() != ""
}

// OnChange 注册挂载状态变更回调（挂载成功、卸载、异常结束）；可注册多个，互不覆盖。
func (m *MountManager) OnChange(fn func(name string, mounted bool, errMsg string)) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, fn)
}

func (m *MountManager) emitChange(name string, mounted bool, errMsg string) {
	m.mu.Lock()
	observers := make([]func(string, bool, string), len(m.observers))
	copy(observers, m.observers)
	m.mu.Unlock()
	for _, fn := range observers {
		fn(name, mounted, errMsg)
	}
}

// friendlyMountError 把 rclone/WinFsp 的底层错误翻译成用户能理解的提示。
func friendlyMountError(target string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "mountpoint path already exists") {
		return fmt.Errorf("挂载点 %s 已被占用，请在驱动器设置中换一个盘符后重试", target)
	}
	return err
}

// Mount 在当前进程中把远程存储挂载到 target（Windows 为盘符，如 "Z:"）。
func (m *MountManager) Mount(remote, target string, readOnly bool) error {
	mountType, mountFn := mountlib.ResolveMountMethod("")
	if mountFn == nil {
		return errors.New("当前构建未启用挂载功能：请使用 -tags cmount 构建，并在 Windows 上安装 WinFsp")
	}
	if err := Ensure(); err != nil {
		return err
	}
	if strings.TrimSpace(target) == "" {
		return errors.New("挂载点不能为空")
	}
	if m.IsMounted(remote) {
		return fmt.Errorf("%s 已挂载", remote)
	}
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("创建挂载目录失败: %w", err)
		}
	}

	f, err := fs.NewFs(context.Background(), remote+":")
	if err != nil {
		return fmt.Errorf("打开远程存储失败: %w", err)
	}

	vfsOpt := vfscommon.Opt
	vfsOpt.CacheMode = vfscommon.CacheModeWrites
	vfsOpt.ReadOnly = readOnly
	mountOpt := mountlib.Opt
	mountOpt.VolumeName = remote

	log := newLogBuffer()
	fmt.Fprintf(log, "[%s] 使用 %s 挂载 %s -> %s (只读=%v)\n", stamp(), mountType, remote, target, readOnly)

	point := mountlib.NewMountPoint(mountFn, target, f, &mountOpt, &vfsOpt)
	if _, err := point.Mount(); err != nil {
		friendly := friendlyMountError(target, err)
		fmt.Fprintf(log, "[%s] 挂载失败: %v\n", stamp(), err)
		return fmt.Errorf("挂载失败: %w", friendly)
	}

	if err := waitMountReady(point, log); err != nil {
		_ = point.Unmount()
		return err
	}

	entry := &mountEntry{point: point, log: log, target: point.MountPoint}
	m.mu.Lock()
	m.mounts[remote] = entry
	m.mu.Unlock()

	go func() {
		err := point.Wait()
		m.mu.Lock()
		delete(m.mounts, remote)
		m.mu.Unlock()
		if err != nil {
			friendly := friendlyMountError(point.MountPoint, err)
			fmt.Fprintf(log, "[%s] 挂载异常结束: %v\n", stamp(), err)
			m.emitChange(remote, false, friendly.Error())
			return
		}
		fmt.Fprintf(log, "[%s] 已卸载\n", stamp())
		m.emitChange(remote, false, "")
	}()

	m.emitChange(remote, true, "")
	return nil
}

// waitMountReady 等待挂载点可访问；Windows 盘符或目录出现即视为就绪。
func waitMountReady(point *mountlib.MountPoint, log *logBuffer) error {
	deadline := time.Now().Add(mountReadyTimeout)
	for {
		select {
		case err := <-point.ErrChan:
			if err == nil {
				err = errors.New("挂载意外结束")
			}
			fmt.Fprintf(log, "[%s] 挂载失败: %v\n", stamp(), err)
			return fmt.Errorf("挂载失败: %w", err)
		case <-time.After(mountReadyInterval):
		}
		if _, err := os.Stat(mountPath(point.MountPoint)); err == nil {
			fmt.Fprintf(log, "[%s] 挂载就绪: %s\n", stamp(), point.MountPoint)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待挂载就绪超时: %s", point.MountPoint)
		}
	}
}

// mountPath 把 Windows 盘符补全为可 stat 的路径。
func mountPath(target string) string {
	if runtime.GOOS == "windows" && len(target) == 2 && target[1] == ':' {
		return target + `\`
	}
	return target
}

// Unmount 卸载指定远程，等待其挂载循环退出。
func (m *MountManager) Unmount(remote string) error {
	m.mu.Lock()
	entry, ok := m.mounts[remote]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("%s 未挂载", remote)
	}
	if err := entry.point.Unmount(); err != nil {
		return fmt.Errorf("卸载失败: %w", err)
	}
	return nil
}

// IsMounted 报告指定远程当前是否已挂载。
func (m *MountManager) IsMounted(remote string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.mounts[remote]
	return ok
}

// ListMounted 返回所有已挂载的远程名称。
func (m *MountManager) ListMounted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.mounts))
	for name := range m.mounts {
		names = append(names, name)
	}
	return names
}

// Log 返回指定远程的挂载日志。
func (m *MountManager) Log(remote string) string {
	m.mu.Lock()
	entry, ok := m.mounts[remote]
	m.mu.Unlock()
	if !ok {
		return ""
	}
	return entry.log.String()
}

// Target 返回指定远程的挂载目标（盘符或目录）。
func (m *MountManager) Target(remote string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.mounts[remote]; ok {
		return entry.target
	}
	return ""
}

// UnmountAll 卸载所有挂载，用于应用退出时清理。
func (m *MountManager) UnmountAll() {
	for _, name := range m.ListMounted() {
		_ = m.Unmount(name)
	}
}
