package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DownloadFileHTTP 使用Go的HTTP客户端下载文件
func DownloadFileHTTP(url, filePath string) error {
	// 创建HTTP客户端，设置超时时间
	client := &http.Client{
		Timeout: 300 * time.Second, // 5分钟超时
	}

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置User-Agent
	userAgent := fmt.Sprintf("%sClient/1.0 (%s; %s)", masquerade(""), runtime.GOOS, runtime.GOARCH)
	req.Header.Set("User-Agent", userAgent)

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	// 创建目录
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	// 创建目标文件
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	// 复制数据
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	return nil
}

// ExecuteDownload 执行下载命令（替代wget）
func ExecuteDownload(url, filePath string) ([]byte, error) {
	// 检查URL格式
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("不支持的URL格式，只支持HTTP/HTTPS: %s", url)
	}

	// 执行下载
	err := DownloadFileHTTP(url, filePath)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %v", err)
	}

	// 获取文件信息
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %v", err)
	}

	successMsg := fmt.Sprintf("下载成功: %s -> %s (大小: %d bytes)", url, filePath, fileInfo.Size())
	return []byte(successMsg), nil
}
