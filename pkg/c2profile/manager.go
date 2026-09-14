package c2profile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ===== Profile 管理器：加载 profiles/*.yaml + 文件监听热重载 =====

var (
	mu        sync.RWMutex
	dir       = "profiles"
	profiles  = make(map[string]*Profile)
	loadedAt  time.Time
	watchStop = make(chan struct{})
)

// Init 初始化 Profile 管理器（幂等）：确保目录与默认 profile 存在后加载，并启动热重载监听。
func Init(profileDir string) error {
	if profileDir != "" {
		dir = profileDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := ensureDefault(); err != nil {
		return err
	}
	if err := Reload(); err != nil {
		return err
	}
	go watchLoop()
	return nil
}

// Reload 重新扫描 profiles 目录（YAML 解析失败跳过并记录）
func Reload() error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	next := make(map[string]*Profile)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			continue
		}
		if p.Name == "" {
			p.Name = strings.TrimSuffix(e.Name(), ".yaml")
		}
		// 默认值兜底
		if p.UserAgent == "" {
			p.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
		}
		if p.SleeptimeMs <= 0 {
			p.SleeptimeMs = 1000
		}
		if p.WS.Path == "" {
			p.WS.Path = "/ws"
		}
		next[p.Name] = &p
	}
	mu.Lock()
	profiles = next
	loadedAt = time.Now()
	mu.Unlock()
	return nil
}

// watchLoop 文件监听热重载（与 config watchConfig 同模式：每 3 秒扫描 mtime 变化）
func watchLoop() {
	lastMtime := map[string]time.Time{}
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-watchStop:
			return
		case <-tick.C:
			changed := false
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			seen := map[string]bool{}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				seen[e.Name()] = true
				mt := info.ModTime()
				if last, ok := lastMtime[e.Name()]; !ok || !last.Equal(mt) {
					lastMtime[e.Name()] = mt
					changed = true
				}
			}
			for name := range lastMtime {
				if !seen[name] {
					delete(lastMtime, name)
					changed = true
				}
			}
			if changed {
				_ = Reload()
			}
		}
	}
}

// List 全部已加载 profile 名
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	return names
}

// Get 按名取 profile（不存在返回 nil）
func Get(name string) *Profile {
	mu.RLock()
	defer mu.RUnlock()
	if name == "" {
		name = "default"
	}
	return profiles[name]
}

// GetDefault 默认 profile
func GetDefault() *Profile {
	return Get("default")
}

// Raw 读取指定 profile 文件原文（预览/编辑用）
func Raw(name string) (string, error) {
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Save 将 YAML 内容写入 profile 文件（供前端保存/校验）
func Save(name string, content string) error {
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return os.ErrInvalid
	}
	var p Profile
	if err := yaml.Unmarshal([]byte(content), &p); err != nil {
		return err
	}
	if p.Name == "" {
		p.Name = name
	}
	out, err := yaml.Marshal(&p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".yaml"), out, 0o644)
}

// ensureDefault 生成默认 profile（含中文注释）
func ensureDefault() error {
	path := filepath.Join(dir, "default.yaml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(defaultProfileYAML), 0o644)
}

// InitNoWatch 初始化 Profile 管理器（仅加载，不启动热重载监听——调试/低资源模式）
func InitNoWatch(profileDir string) error {
	if profileDir != "" {
		dir = profileDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := ensureDefault(); err != nil {
		return err
	}
	return Reload()
}
