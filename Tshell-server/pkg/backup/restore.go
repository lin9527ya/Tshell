package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"BackendTemplate/pkg/utils/logx"
)

// Restore 用备份替换当前数据库文件。
//
// 只能在服务端**未运行**时调用（见 cmd 层的 -restore）：SQLite 的数据库就是那个
// 文件本身，运行中的进程持有它、还有 -wal/-shm 兄弟文件，热替换会把两边都弄坏。
// 因此这里做三件事：
//  1. 先校验备份（不可用的备份绝不能覆盖现有数据）；
//  2. 把现有数据库另存为 <db>.pre-restore-<时间>（恢复错了还能退回去）；
//  3. 临时文件 + rename 原子落位，并清掉旧的 -wal/-shm。
func Restore(backupPath, dbPath string, force bool) (safetyCopy string, err error) {
	if backupPath == "" || dbPath == "" {
		return "", fmt.Errorf("备份路径与数据库路径都不能为空")
	}
	if err := Verify(backupPath); err != nil {
		return "", fmt.Errorf("备份不可用，已放弃恢复: %w", err)
	}
	if same, _ := sameFile(backupPath, dbPath); same {
		return "", fmt.Errorf("备份文件与目标数据库是同一个文件")
	}

	// 运行中的实例会留下非空 WAL（干净关闭时 SQLite 会 checkpoint 并删除它）
	if !force {
		if st, statErr := os.Stat(dbPath + "-wal"); statErr == nil && st.Size() > 0 {
			return "", fmt.Errorf("检测到 %s-wal 非空，数据库可能正在被服务端使用；请先停止服务端（或加 -force 强制继续）", dbPath)
		}
	}

	if _, statErr := os.Stat(dbPath); statErr == nil {
		safetyCopy = fmt.Sprintf("%s.pre-restore-%s", dbPath, time.Now().Format(nameTimeLayout))
		if err := copyFile(dbPath, safetyCopy, 0o600); err != nil {
			return "", fmt.Errorf("保存恢复前快照失败（未改动现有数据）: %w", err)
		}
		logx.Infof("🗂️ [恢复] 恢复前快照: %s\n", safetyCopy)
	}

	// 同目录内写临时文件再 rename：避免跨设备且保证原子替换
	tmp := dbPath + ".restore.tmp"
	if err := copyFile(backupPath, tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return safetyCopy, fmt.Errorf("写入数据库失败: %w", err)
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		_ = os.Remove(tmp)
		return safetyCopy, fmt.Errorf("替换数据库失败: %w", err)
	}
	// 旧 WAL/SHM 属于被替换掉的旧库，必须清掉，否则新库会读到不匹配的日志
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}
	return safetyCopy, nil
}

// copyFile 复制文件（写入后 fsync，避免恢复出来的库还在页缓存里就断电）
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// sameFile 判断两个路径是否指向同一个文件
func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err // 目标不存在时不算同一个文件
	}
	return os.SameFile(sa, sb), nil
}

// ResolveBackup 把用户给的备份参数解析成备份文件路径：
// 既接受完整路径，也接受备份目录里的文件名（CLI 用）。
func ResolveBackup(m *Manager, arg string) (string, error) {
	if arg == "" {
		return "", fmt.Errorf("未指定备份文件")
	}
	if _, err := os.Stat(arg); err == nil {
		return arg, nil
	}
	name := filepath.Base(arg)
	path, err := m.resolve(name)
	if err != nil {
		return "", fmt.Errorf("找不到备份 %q（既不是现有文件，也不是备份目录里的备份名）", arg)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("找不到备份 %q", arg)
	}
	return path, nil
}
