package security

import (
	"os"
	"path/filepath"
	"runtime"
)

// ===== 文件系统权限收敛（OPSEC P0-4）=====
//
// 背景：项目里多处用 os.MkdirAll(dir, os.ModePerm)（=0777）建目录，
// 会话录像用 0644 建文件。这些目录/文件里放的是**战果与本体会话内容**：
//   Downloads/<uid>/   客户端下载回来的文件
//   plugins/           上传的插件载荷
//   sessions/          交互式 shell 录像（可能含操作员输入的口令）
//   xterm/            终端会话
//
// 0777/0644 意味着同机任何本地用户都能读走这些东西。C2 服务端通常独占一台机器，
// 但"通常"不是保证 —— 旁路账号、备份代理、日志采集器、共享宿主（容器）都是实际存在的读者。
//
// 这里的约定：**目录 0700、含凭据或会话内容的文件 0600**。
// 与项目里原本就正确的做法保持一致（备份与审计日志一直用 0700/0600）。

const (
	// DirMode 私有目录权限
	DirMode os.FileMode = 0o700
	// FileMode 敏感文件权限
	FileMode os.FileMode = 0o600
)

// permsSupported 判断当前平台是否体现 POSIX 权限位。
// Windows 上 os.Chmod 只能改只读位，收敛动作无意义但也不应报错。
func permsSupported() bool { return runtime.GOOS != "windows" }

// needsTighten 判断当前权限是否"比目标更宽"。
//
// 抽成纯函数的原因：os.Chmod 的语义在 Windows 上只体现只读位，
// 权限相关用例在 Windows 只能 skip —— 那等于本机没有任何证据。
// 把判定独立出来后，至少"该不该收紧、会不会把更严的权限放宽"
// 这两条关键决策在**所有平台**都有真测试覆盖；
// 实际 chmod 生效与否由 CI 的 ubuntu job 覆盖。
func needsTighten(cur, want os.FileMode) bool {
	return cur&^want != 0
}

// TightenIfWider 把已存在路径的权限收窄到 want；已足够严格则不动。
// 返回是否发生了变更（便于调用方只在确实改过时记日志/审计）。
func TightenIfWider(path string, want os.FileMode) (bool, error) {
	if !permsSupported() {
		return false, nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	// 只处理"比目标更宽"的情况：不要把运维有意设置的更严权限放宽
	if !needsTighten(st.Mode().Perm(), want) {
		return false, nil
	}
	if err := os.Chmod(path, want); err != nil {
		return false, err
	}
	return true, nil
}

// SecureMkdirAll 以 0700 创建目录树。
//
// MkdirAll 只会给**新建**的目录设权限，已存在的目录保持原样 —— 历史部署里
// 那些 0777 目录不会自动变好，所以这里补一次收敛（best-effort：
// 收敛失败不阻断业务，由调用方决定是否告警）。
func SecureMkdirAll(path string) error {
	if err := os.MkdirAll(path, DirMode); err != nil {
		return err
	}
	_, _ = TightenIfWider(path, DirMode)
	return nil
}

// SecureCreate 以 0600 创建（或截断）文件。
//
// 先 OpenFile(O_CREATE, 0600) 再写，而不是"先按默认权限写再 chmod"：
// 后者中间有一段窗口是默认权限，正好是敏感内容已落盘、权限还没收的时刻。
func SecureCreate(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, FileMode)
	if err != nil {
		return nil, err
	}
	// 已存在文件的权限不受 O_CREATE 影响，显式收敛
	_, _ = TightenIfWider(path, FileMode)
	return f, nil
}

// SecureAppend 以 0600 追加打开文件（会话录像这类边写边读的场景）
func SecureAppend(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), DirMode); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, FileMode)
	if err != nil {
		return nil, err
	}
	_, _ = TightenIfWider(path, FileMode)
	return f, nil
}

// TightenDataDirs 启动时对项目自己管理的目录做一次权限收敛。
//
// 返回被改动的路径清单，供调用方记一条审计 —— 静默改权限不合适，
// 运维需要知道"这台机器上哪些目录被收紧了"，否则排查共享访问问题时毫无头绪。
func TightenDataDirs(dirs ...string) []string {
	if !permsSupported() {
		return nil
	}
	var changed []string
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if _, err := os.Stat(d); err != nil {
			continue // 不存在就不管：不存在比权限宽更安全
		}
		if ok, err := TightenIfWider(d, DirMode); err != nil {
			continue
		} else if ok {
			changed = append(changed, d)
		}
	}
	return changed
}
