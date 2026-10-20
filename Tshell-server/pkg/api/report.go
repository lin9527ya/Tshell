package api

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/security"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== C15: 操作报告导出 =====
// GET /api/client/report/:uid 导出客户端操作报告（shell 历史 + 会话录像列表 + 下载记录）

// ClientReport 客户端操作报告
type ClientReport struct {
	UID          string               `json:"uid"`
	GeneratedAt  string               `json:"generated_at"`
	ShellHistory string               `json:"shell_history"`
	SessionLogs  []SessionLogInfo     `json:"session_logs"`
	Downloads    []database.Downloads `json:"downloads"`
}

// SessionLogInfo 会话录像信息
type SessionLogInfo struct {
	File         string `json:"file"`
	Size         int64  `json:"size"`
	Modified     string `json:"modified"`
	Downloadable bool   `json:"downloadable"`
}

// GetClientReport 导出客户端操作报告
func GetClientReport(c *gin.Context) {
	uid := c.Param("uid")
	if uid == "" {
		uid = c.Query("uid")
	}
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UID不能为空"})
		return
	}

	report := ClientReport{
		UID:         uid,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	// 1. Shell 历史
	var shell database.Shell
	shell, _, _ = database.Shells().GetByUID(uid)
	report.ShellHistory = shell.ShellContent

	// 2. 会话录像列表
	sessDir := "./sessions"
	if entries, err := os.ReadDir(sessDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			// 文件名格式: <uid>_<ts>.log
			if len(name) > len(uid) && name[:len(uid)] == uid {
				info, _ := e.Info()
				report.SessionLogs = append(report.SessionLogs, SessionLogInfo{
					File:         name,
					Size:         info.Size(),
					Modified:     info.ModTime().Format("2006-01-02 15:04:05"),
					Downloadable: true,
				})
			}
		}
		sort.Slice(report.SessionLogs, func(i, j int) bool {
			return report.SessionLogs[i].File > report.SessionLogs[j].File
		})
	}

	// 3. 下载记录
	report.Downloads, _ = database.DownloadsStore_().ListByUID(uid)

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": report})
}

// DownloadSessionLog 下载指定会话录像文件
func DownloadSessionLog(c *gin.Context) {
	uid := c.Query("uid")
	filename := c.Query("file")
	if uid == "" || filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uid/file 必填"})
		return
	}
	// 路径安全：仅允许 sessions 目录内且前缀匹配 uid 的文件
	base := "./sessions"
	clean := filepath.Base(filename)
	if len(clean) <= len(uid) || clean[:len(uid)] != uid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法文件名"})
		return
	}
	path := filepath.Join(base, clean)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "录像文件不存在"})
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+clean)
	c.File(path)
}

// ===== 报告补齐:威胁指标 / 社会工程 / 战术 TTP =====

// IOCReport 威胁指标报告
type IOCReport struct {
	GeneratedAt       string        `json:"generated_at"`
	Hosts             []IOCSession  `json:"hosts"`
	ListenerAddresses []string      `json:"listener_addresses"`
	LoginFailures     []string      `json:"login_failures"`
	BlockedIPs        []string      `json:"blocked_ips"`
	Artifacts         []IOCArtifact `json:"artifacts"`
}

// IOCSession 会话指标
type IOCSession struct {
	UID        string `json:"uid"`
	Host       string `json:"host"`
	User       string `json:"user"`
	ExternalIP string `json:"external_ip"`
	InternalIP string `json:"internal_ip"`
	Process    string `json:"process"`
	Arch       string `json:"arch"`
	Listener   string `json:"listener"`
	Online     string `json:"online"`
}

// IOCArtifact 投递产物
type IOCArtifact struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// GetIOCReport 威胁指标报告:聚合会话/监听器/审计失败/封禁 IP/投递产物
func GetIOCReport(c *gin.Context) {
	rep := IOCReport{GeneratedAt: time.Now().Format("2006-01-02 15:04:05"), Hosts: []IOCSession{}, ListenerAddresses: []string{}, LoginFailures: []string{}, BlockedIPs: []string{}, Artifacts: []IOCArtifact{}}

	if clients, err := database.Clients_().All(); err == nil {
		for _, cl := range clients {
			rep.Hosts = append(rep.Hosts, IOCSession{
				UID: cl.Uid, Host: cl.Computer, User: cl.Username,
				ExternalIP: cl.ExternalIP, InternalIP: cl.InternalIP,
				Process: cl.Process, Arch: cl.Arch, Listener: cl.Listener, Online: cl.Online,
			})
		}
	}
	if listeners, err := database.Listeners().List(); err == nil {
		seen := map[string]bool{}
		for _, l := range listeners {
			addr := l.ConnectAddress
			if addr == "" {
				addr = l.ListenAddress
			}
			key := l.Type + "://" + addr
			if !seen[key] {
				seen[key] = true
				rep.ListenerAddresses = append(rep.ListenerAddresses, key)
			}
		}
	}
	if downloads, err := database.DownloadsStore_().ListAll(); err == nil {
		seen := map[string]bool{}
		for _, d := range downloads {
			if !seen[d.FileName] {
				seen[d.FileName] = true
				rep.Artifacts = append(rep.Artifacts, IOCArtifact{Name: d.FileName, Size: int64(d.FileSize)})
			}
		}
	}
	// 审计:登录失败（最近 7 天，最多 50 条；从落盘审计日志取，与 /api/audit/logs 同源）
	// 原实现把 LoginFailures 初始化为空数组后再也没填充过，报告里这个字段恒为空。
	if fails := audit.QueryLoginFailures(time.Now().Add(-7*24*time.Hour), 50); len(fails) > 0 {
		rep.LoginFailures = fails
	}
	// 封禁 IP
	if security.GlobalRateLimiter != nil {
		for _, b := range security.GlobalRateLimiter.GetBlockedIPs() {
			if ip, ok := b["ip"].(string); ok {
				rep.BlockedIPs = append(rep.BlockedIPs, ip)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": rep})
}

// SocialReport 社会工程报告
type SocialReport struct {
	GeneratedAt string           `json:"generated_at"`
	Deliveries  []SocialDelivery `json:"deliveries"`
	Clicks      []SocialClick    `json:"clicks"`
	TotalClicks int              `json:"total_clicks"`
	UniqueIPs   int              `json:"unique_ips"`
}

type SocialDelivery struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

type SocialClick struct {
	Time       string `json:"time"`
	Resource   string `json:"resource"`
	ClientIP   string `json:"client_ip"`
	UserAgent  string `json:"user_agent"`
	StatusCode int    `json:"status_code"`
}

// GetSocialReport 社会工程报告:基于 WebDelivery 投递访问日志
func GetSocialReport(c *gin.Context) {
	rep := SocialReport{GeneratedAt: time.Now().Format("2006-01-02 15:04:05"), Deliveries: []SocialDelivery{}, Clicks: []SocialClick{}}

	if wds, err := database.Deliveries().List(); err == nil {
		for _, w := range wds {
			st := "关闭"
			if w.Status == 1 {
				st = "开启"
			}
			rep.Deliveries = append(rep.Deliveries, SocialDelivery{
				Name: w.FileName, URL: w.ServerAddress, Status: st,
			})
		}
	}
	if logs, err := database.DeliveryLogs().Recent(500); err == nil {
		ips := map[string]bool{}
		for _, l := range logs {
			rep.Clicks = append(rep.Clicks, SocialClick{
				Time:     l.CreatedAt.Format("2006-01-02 15:04:05"),
				Resource: l.Resource, ClientIP: l.ClientIP,
				UserAgent: l.UserAgent, StatusCode: l.StatusCode,
			})
			ips[l.ClientIP] = true
		}
		rep.TotalClicks = len(logs)
		rep.UniqueIPs = len(ips)
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": rep})
}

// TTPReport 战术 TTP 报告
type TTPReport struct {
	GeneratedAt string         `json:"generated_at"`
	PerSession  []TTPLine      `json:"per_session"`
	Techniques  []TTPTechnique `json:"techniques"`
}

type TTPLine struct {
	Host      string `json:"host"`
	User      string `json:"user"`
	Command   string `json:"command"`
	Tactic    string `json:"tactic"`
	Technique string `json:"technique"`
}

type TTPTechnique struct {
	Tactic    string `json:"tactic"`
	Technique string `json:"technique"`
	Count     int    `json:"count"`
}

// ttpMap 命令首词 → (战术, 技术) 基础映射(MITRE ATT&CK 简化)
var ttpMap = map[string][2]string{
	"persist":         {"持久化", "T1547 启动项/计划任务"},
	"wmi_persist":     {"持久化", "T1546 WMI 事件订阅"},
	"psexec":          {"横向移动", "T1021 SMB 服务执行"},
	"wmi_exec":        {"横向移动", "T1047 WMI 远程执行"},
	"winrm_exec":      {"横向移动", "T1021 WinRM"},
	"make_token":      {"凭证获取", "T1134 令牌模拟"},
	"steal_token":     {"凭证获取", "T1134 令牌窃取"},
	"getsystem":       {"提权", "T1134 令牌操纵 / T1548.002 UAC 绕过"},
	"inject":          {"防御规避", "T1055 进程注入"},
	"keylog_start":    {"收集", "T1056 键盘记录"},
	"keylog_get":      {"收集", "T1056 键盘记录"},
	"screenshot1":     {"收集", "T1113 屏幕截图"},
	"clipboard":       {"收集", "T1115 剪贴板"},
	"ad_enum":         {"侦察", "T1018 域枚举"},
	"net":             {"侦察", "T1018 网络发现"},
	"shell":           {"执行", "T1059 命令解释器"},
	"execute":         {"执行", "T1059 程序执行"},
	"selfdelete":      {"防御规避", "T1070 痕迹清理"},
	"selfdelete_exit": {"防御规避", "T1070 痕迹清理"},
	"cleanup":         {"防御规避", "T1070 痕迹清理"},
	"etw_patch":       {"防御规避", "T1562 禁用检测"},
	"unhook_ntdll":    {"防御规避", "T1562 禁用检测"},
	"evasion":         {"防御规避", "T1562 禁用检测"},
	"hideprocess":     {"防御规避", "T1564 伪装"},
	"hidenet":         {"防御规避", "T1564 伪装"},
	"reg_query":       {"发现", "T1012 注册表查询"},
	"services":        {"发现", "T1007 服务发现"},
}

// GetTTPReport 战术 TTP 报告:解析会话 shell 历史命令 → MITRE 映射统计
func GetTTPReport(c *gin.Context) {
	rep := TTPReport{GeneratedAt: time.Now().Format("2006-01-02 15:04:05"), PerSession: []TTPLine{}, Techniques: []TTPTechnique{}}

	clients, _ := database.Clients_().All()
	techniqueCount := map[string]int{}
	for _, cl := range clients {
		sh, has, _ := database.Shells().GetByUID(cl.Uid)
		if !has {
			continue
		}
		for _, line := range splitShellLines(sh.ShellContent) {
			words := splitCommandWords(line)
			if len(words) == 0 {
				continue
			}
			key := words[0]
			if len(words) > 1 && words[0] == "$" {
				key = words[1] // 格式 "$ cmd ..."
			}
			tactic, technique := classifyTTP(key)
			if tactic == "" {
				continue
			}
			rep.PerSession = append(rep.PerSession, TTPLine{
				Host: cl.Computer, User: cl.Username, Command: line,
				Tactic: tactic, Technique: technique,
			})
			techniqueCount[tactic+"|"+technique]++
		}
	}
	for k, v := range techniqueCount {
		parts := strings.SplitN(k, "|", 2)
		rep.Techniques = append(rep.Techniques, TTPTechnique{
			Tactic: parts[0], Technique: parts[1], Count: v,
		})
	}
	sort.Slice(rep.Techniques, func(i, j int) bool {
		return rep.Techniques[i].Count > rep.Techniques[j].Count
	})
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": rep})
}

func classifyTTP(cmd string) (tactic, technique string) {
	if m, ok := ttpMap[cmd]; ok {
		return m[0], m[1]
	}
	switch cmd {
	case "cd", "ls", "pwd", "dir", "mkdir", "rm", "cp", "mv", "download", "upload":
		return "发现", "T1083 文件目录发现"
	case "ps", "kill":
		return "发现", "T1057 进程发现"
	case "wifi":
		return "凭证获取", "T1021 凭据转储"
	case "drives":
		return "发现", "T1082 系统信息发现"
	case "psexec_psh", "smbexec":
		return "横向移动", "T1021 SMB"
	}
	return "", ""
}

func splitShellLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func splitCommandWords(s string) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	return fields
}
