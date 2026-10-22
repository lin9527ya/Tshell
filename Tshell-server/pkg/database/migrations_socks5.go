package database

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"BackendTemplate/pkg/utils/logx"
)

// migrateLegacySocks5ToTunnels 把旧版 SOCKS5 代理（socks5 表）并入统一隧道表 tunnel_proxy。
//
// 背景：早期用的是 /socks5/* + Socks5 表；后来统一到 /tunnel-proxy/* + TunnelProxy 表
// （protocol=socks5）。两套实现下发的是同一条客户端命令，属于重复实现。这里只做数据
// 迁移，不删除旧表——迁移失败或需要回滚时原始数据仍在，确认无误后可由运维手工清理。
//
// 幂等：按 server_port 去重（tunnel_proxy.server_port 本身是唯一列），重复执行不会产生副本。
func migrateLegacySocks5ToTunnels() error {
	if Engine == nil {
		return nil
	}
	exists, err := Engine.IsTableExist("socks5")
	if err != nil {
		return fmt.Errorf("检查 socks5 表失败: %w", err)
	}
	if !exists {
		return nil // 新库：从未有过旧表
	}
	hasTunnel, err := Engine.IsTableExist("tunnel_proxy")
	if err != nil {
		return fmt.Errorf("检查 tunnel_proxy 表失败: %w", err)
	}
	if !hasTunnel {
		return nil // Sync2 应先建表；没建就等下次启动
	}

	rows, err := QueryStringSQL("SELECT * FROM socks5")
	if err != nil {
		return fmt.Errorf("读取旧 SOCKS5 记录失败: %w", err)
	}
	migrated, skipped := 0, 0
	for _, row := range rows {
		// QueryString 的键名是数据库里的真实列名；不同历史版本存在 uid / u_i_d 两种写法。
		portStr := strings.TrimSpace(firstNonEmpty(row, "socks5port", "socks5_port", "port"))
		port, perr := strconv.Atoi(portStr)
		if perr != nil || port <= 0 || port > 65535 {
			skipped++
			continue
		}
		protocol := strings.ToLower(strings.TrimSpace(firstNonEmpty(row, "type", "protocol")))
		if protocol == "" {
			protocol = "socks5"
		}
		if protocol != "socks5" {
			// 旧表只承载 SOCKS5；其他值属于脏数据，不迁移
			skipped++
			continue
		}
		// 端口已被新表占用（人工建过同一端口）时保留新表记录
		dup, derr := QueryStringSQL("SELECT id FROM tunnel_proxy WHERE server_port = ? LIMIT 1", port)
		if derr != nil {
			return fmt.Errorf("检查端口 %d 是否已迁移失败: %w", port, derr)
		}
		if len(dup) > 0 {
			skipped++
			continue
		}

		status := 0
		if strings.TrimSpace(firstNonEmpty(row, "status")) == "1" {
			status = 1
		}
		created := parseLegacyTime(firstNonEmpty(row, "created_at"))
		updated := parseLegacyTime(firstNonEmpty(row, "updated_at"))
		if created.IsZero() {
			created = time.Now()
		}
		if updated.IsZero() {
			updated = created
		}
		if _, ierr := ExecSQL(
			`INSERT INTO tunnel_proxy
			 (client_uid, protocol, server_port, target_address, note, username, password, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			firstNonEmpty(row, "uid", "u_i_d", "client_uid"),
			protocol,
			port,
			"",
			firstNonEmpty(row, "note"),
			firstNonEmpty(row, "user_name", "username"),
			firstNonEmpty(row, "password"),
			status,
			created,
			updated,
		); ierr != nil {
			return fmt.Errorf("迁移旧 SOCKS5 记录（端口 %d）失败: %w", port, ierr)
		}
		migrated++
	}
	if migrated > 0 || skipped > 0 {
		logx.Infof("[迁移] 旧 SOCKS5 代理并入 tunnel_proxy：迁移 %d 条，跳过 %d 条\n", migrated, skipped)
	}
	return nil
}

// firstNonEmpty 从 QueryString 的结果里按候选列名取第一个非空值。
func firstNonEmpty(row map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != "" {
			return v
		}
	}
	return ""
}

// parseLegacyTime 解析旧库里的时间字符串（SQLite 与 PostgreSQL 的常见输出格式）。
func parseLegacyTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
