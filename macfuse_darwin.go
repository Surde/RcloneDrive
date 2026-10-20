//go:build darwin

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// macFUSE 官方发布（版本固定，校验和来自官方 .sha256 附属文件）。
// 安装与批准后由内核按需加载，应用不打包 macFUSE 本体。
const (
	macfuseVersion = "5.4.0"
	macfuseURL     = "https://github.com/macfuse/macfuse/releases/download/macfuse-5.4.0/macfuse-5.4.0.dmg"
	macfuseSHA256  = "861814f0ac7fa8f6547ea40cdd49a36ac84bcc7d34f38a1fa74e8cf68b0401c5"
)

// macfuseInstalled 报告 macFUSE 是否已安装到系统位置。
func macfuseInstalled() bool {
	_, err := os.Stat("/Library/Filesystems/macfuse.fs")
	return err == nil
}

var macfuseUsableCache struct {
	sync.Mutex
	ok     bool
	probed time.Time
}

const macfuseUsableCacheTTL = 15 * time.Second

// macfuseUsable 报告 macFUSE 是否已可用（系统扩展已启用或内核扩展已加载）。
// 4.x 是 kext（kextstat），5.x 是 system extension（systemextensionsctl），
// 两者都查；查询结果短暂缓存。
func macfuseUsable() bool {
	macfuseUsableCache.Lock()
	defer macfuseUsableCache.Unlock()
	if time.Since(macfuseUsableCache.probed) < macfuseUsableCacheTTL {
		return macfuseUsableCache.ok
	}
	macfuseUsableCache.probed = time.Now()
	macfuseUsableCache.ok = probeMacfuseUsable()
	return macfuseUsableCache.ok
}

func probeMacfuseUsable() bool {
	out, _ := exec.Command("kextstat", "-l").Output()
	if strings.Contains(strings.ToLower(string(out)), "macfuse") {
		return true
	}
	out, _ = exec.Command("systemextensionsctl", "list").Output()
	low := strings.ToLower(string(out))
	if !strings.Contains(low, "macfuse") {
		return false
	}
	// 列出 macfuse 且存在 enabled 行即认为已批准
	for _, line := range strings.Split(low, "\n") {
		if strings.Contains(line, "macfuse") && strings.Contains(line, "enabled") {
			return true
		}
	}
	// 有的版本 enabled 状态列在独立行（enabled\tteamID\tbundleID），宽松判断
	return strings.Contains(low, "enabled")
}

// macfuseApprovalNeededErr 是已安装但尚未批准（或刚装完等待批准）时的引导错误。
func macfuseApprovalNeededErr() error {
	return fmt.Errorf("macFUSE 已安装，还需一次性手动批准：打开 系统设置 → 隐私与安全性，" +
		"允许 macFUSE 系统扩展（Apple 芯片可能需在恢复模式中允许降低安全性），" +
		"批准并重启应用后再挂载")
}

var macfuseInstallMu sync.Mutex

// ensureMacFUSE 挂载前确保 macFUSE 可用：未安装时下载官方 dmg 并经
// osascript 请求管理员权限安装；安装后引导用户完成系统扩展批准
// （苹果安全设计，无法代劳）。
func ensureMacFUSE() error {
	if macfuseInstalled() {
		if macfuseUsable() {
			return nil
		}
		return macfuseApprovalNeededErr()
	}

	macfuseInstallMu.Lock()
	defer macfuseInstallMu.Unlock()
	if macfuseInstalled() {
		if macfuseUsable() {
			return nil
		}
		return macfuseApprovalNeededErr()
	}

	notifyUserTo("main", "info", "需要安装 macFUSE",
		"首次挂载云盘需要 macFUSE 文件系统组件，将下载官方安装包并请求管理员权限，请在弹出的系统对话框中授权。")
	log.Printf("未检测到 macFUSE，开始下载安装（%s）", macfuseVersion)

	dmg, err := downloadMacfuse()
	if err != nil {
		return err
	}
	defer os.Remove(dmg)

	if err := installMacfuse(dmg); err != nil {
		return fmt.Errorf("macFUSE 安装失败或被用户取消: %w", err)
	}
	log.Printf("macFUSE 安装完成，等待系统扩展批准")
	notifyUserTo("main", "info", "macFUSE 安装完成", "还需在 系统设置 → 隐私与安全性 中允许 macFUSE，批准后重启应用再挂载。")
	return macfuseApprovalNeededErr()
}

// downloadMacfuse 下载官方 dmg 并做 SHA-256 校验（和来自官方 .sha256 文件）。
func downloadMacfuse() (string, error) {
	req, err := http.NewRequest("GET", macfuseURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute, Transport: updateTransport()}).Do(req)
	if err != nil {
		return "", fmt.Errorf("下载 macFUSE 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 macFUSE 失败: HTTP %d", resp.StatusCode)
	}
	path := filepath.Join(os.TempDir(), "macfuse-"+macfuseVersion+".dmg")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("下载 macFUSE 失败: %w", err)
	}
	f.Close()
	if sum := hex.EncodeToString(h.Sum(nil)); sum != macfuseSHA256 {
		os.Remove(path)
		return "", fmt.Errorf("macFUSE 校验和不匹配（%s），已删除，请重试", sum)
	}
	return path, nil
}

// installMacfuse 挂载 dmg 找到 pkg，经 osascript 一次性管理员授权安装。
// 卷名/包名随版本变化，挂载点与 pkg 路径都从命令输出解析，不写死。
func installMacfuse(dmg string) error {
	attach := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", dmg)
	out, err := attach.Output()
	if err != nil {
		return fmt.Errorf("挂载 macFUSE 镜像失败: %w", err)
	}
	// 输出末行末列是挂载点
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return fmt.Errorf("挂载 macFUSE 镜像失败: 无法解析挂载点")
	}
	mnt := fields[len(fields)-1]
	defer exec.Command("hdiutil", "detach", mnt).Run()

	pkgOut, err := exec.Command("sh", "-c",
		"find '"+mnt+"' -maxdepth 2 -name '*.pkg' | head -1").Output()
	pkg := strings.TrimSpace(string(pkgOut))
	if err != nil || pkg == "" {
		return fmt.Errorf("在镜像中未找到安装包（%s）", mnt)
	}

	// 单次管理员授权完成整个安装；用户取消授权时 osascript 非零退出
	script := fmt.Sprintf(
		`do shell script "installer -pkg %q -target /" with administrator privileges`,
		pkg)
	cmd := exec.Command("osascript", "-e", script)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
