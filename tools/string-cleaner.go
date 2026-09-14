package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 敏感信息模式
var sensitivePatterns = []string{
	`go\d+\.\d+\.\d+`,                                    // Go版本信息
	`golang\.org/x/[a-zA-Z0-9/]+`,                       // Go标准库路径
	`github\.com/[a-zA-Z0-9_\-]+/[a-zA-Z0-9_\-/]+`,     // GitHub依赖路径
	`[A-Za-z]:\\Users\\[^\\]+\\[^\\]+`,                  // Windows用户路径
	`/Users/[^/]+/[^/]+`,                                // macOS用户路径
	`/home/[^/]+/[^/]+`,                                 // Linux用户路径
	`\.go:\d+`,                                          // Go源文件行号
	`runtime\.[a-zA-Z_]+`,                               // Runtime函数
	`src/runtime/[a-zA-Z0-9_\.]+`,                       // Runtime源文件
	`build\s+\-[a-zA-Z]+`,                               // 构建参数
	`\-ldflags.*`,                                       // ldflags参数
	`CGO_ENABLED=\d`,                                    // CGO环境变量
	`GOOS=[a-zA-Z]+`,                                    // GOOS环境变量
	`GOARCH=[a-zA-Z0-9]+`,                               // GOARCH环境变量
}

// 替换字符串模式
var replacements = map[string]string{
	"Reacon":    "Service",
	"Tshell":    "Manager", 
	"listener":  "handler",
	"client":    "worker",
	"server":    "daemon",
	"backdoor":  "channel",
	"payload":   "data",
	"exploit":   "process",
	"malware":   "software",
	"trojan":    "module",
	"keylog":    "monitor",
	"reverse":   "forward",
	"shell":     "terminal",
	"cmd":       "exec",
	"execute":   "run",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run string-cleaner.go <二进制文件路径>")
		fmt.Println("")
		fmt.Println("功能:")
		fmt.Println("  • 检测二进制文件中的敏感字符串")
		fmt.Println("  • 生成清理后的文件")
		fmt.Println("  • 混淆关键词")
		fmt.Println("")
		fmt.Println("示例:")
		fmt.Println("  go run string-cleaner.go ./tshell-server.exe")
		fmt.Println("  go run string-cleaner.go ./release/")
		os.Exit(1)
	}

	target := os.Args[1]
	
	// 检查是文件还是目录
	info, err := os.Stat(target)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		// 处理目录中的所有文件
		processDirectory(target)
	} else {
		// 处理单个文件
		processFile(target)
	}
}

// 处理目录
func processDirectory(dirPath string) {
	fmt.Printf("处理目录: %s\n", dirPath)
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// 只处理可执行文件
		if !info.IsDir() && isExecutable(path) {
			processFile(path)
		}
		
		return nil
	})
	
	if err != nil {
		fmt.Printf("处理目录时出错: %v\n", err)
	}
}

// 检查是否为可执行文件
func isExecutable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".exe" || ext == "" // Windows可执行文件或Linux/macOS无扩展名文件
}

// 处理单个文件
func processFile(filePath string) {
	fmt.Printf("\n=== 处理文件: %s ===\n", filePath)
	
	// 检测敏感信息
	sensitiveCount := detectSensitiveStrings(filePath)
	if sensitiveCount == 0 {
		fmt.Println("✅ 未发现敏感信息")
		return
	}
	
	fmt.Printf("🔍 发现 %d 个敏感信息，开始清理...\n", sensitiveCount)
	
	// 创建清理后的文件
	cleanedPath := createCleanedFile(filePath)
	if cleanedPath != "" {
		// 验证清理效果
		afterCount := detectSensitiveStrings(cleanedPath)
		fmt.Printf("✨ 清理完成: %s\n", cleanedPath)
		fmt.Printf("📊 清理前: %d 个敏感信息\n", sensitiveCount)
		fmt.Printf("📊 清理后: %d 个敏感信息\n", afterCount)
		
		if afterCount < sensitiveCount {
			fmt.Println("✅ 清理效果良好")
		} else {
			fmt.Println("⚠️  清理效果有限")
		}
	}
}

// 检测敏感字符串
func detectSensitiveStrings(filePath string) int {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("无法打开文件: %v\n", err)
		return 0
	}
	defer file.Close()
	
	// 编译正则表达式
	var patterns []*regexp.Regexp
	for _, pattern := range sensitivePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			fmt.Printf("正则表达式编译失败: %s - %v\n", pattern, err)
			continue
		}
		patterns = append(patterns, re)
	}
	
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanBytes)
	
	var content strings.Builder
	for scanner.Scan() {
		content.WriteByte(scanner.Bytes()[0])
	}
	
	contentStr := content.String()
	sensitiveCount := 0
	foundPatterns := make(map[string]bool)
	
	for i, pattern := range patterns {
		matches := pattern.FindAllString(contentStr, -1)
		if len(matches) > 0 {
			patternName := sensitivePatterns[i]
			if !foundPatterns[patternName] {
				fmt.Printf("🚨 发现敏感模式: %s (%d 次匹配)\n", patternName, len(matches))
				foundPatterns[patternName] = true
				sensitiveCount += len(matches)
			}
		}
	}
	
	return sensitiveCount
}

// 创建清理后的文件
func createCleanedFile(filePath string) string {
	// 读取原文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("读取文件失败: %v\n", err)
		return ""
	}
	
	// 创建副本
	ext := filepath.Ext(filePath)
	base := strings.TrimSuffix(filePath, ext)
	cleanedPath := base + "_cleaned" + ext
	
	// 应用字符串替换
	cleanedContent := applyStringReplacements(content)
	
	// 写入清理后的文件
	err = os.WriteFile(cleanedPath, cleanedContent, 0755)
	if err != nil {
		fmt.Printf("写入清理文件失败: %v\n", err)
		return ""
	}
	
	return cleanedPath
}

// 应用字符串替换
func applyStringReplacements(content []byte) []byte {
	contentStr := string(content)
	
	// 应用替换规则
	for old, new := range replacements {
		contentStr = strings.ReplaceAll(contentStr, old, new)
		// 同时替换大写版本
		contentStr = strings.ReplaceAll(contentStr, strings.ToUpper(old), strings.ToUpper(new))
	}
	
	// 使用正则表达式清理敏感模式
	for _, pattern := range sensitivePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		
		// 替换为随机字符串
		contentStr = re.ReplaceAllStringFunc(contentStr, func(match string) string {
			return generateRandomString(len(match))
		})
	}
	
	return []byte(contentStr)
}

// 生成随机字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[i%len(charset)]
	}
	return string(result)
}

// 显示帮助信息
func showUsage() {
	fmt.Println("Tshell 字符串清理工具")
	fmt.Println("")
	fmt.Println("功能:")
	fmt.Println("  • 检测和清理二进制文件中的敏感字符串")
	fmt.Println("  • 混淆关键词和路径信息")
	fmt.Println("  • 生成清理后的文件副本")
	fmt.Println("")
	fmt.Println("支持的敏感信息类型:")
	fmt.Println("  • Go版本和构建信息")
	fmt.Println("  • 源代码路径")
	fmt.Println("  • 依赖库路径")
	fmt.Println("  • 用户目录路径")
	fmt.Println("  • 关键词和函数名")
	fmt.Println("")
	fmt.Println("注意:")
	fmt.Println("  • 清理后的文件可能无法正常运行")
	fmt.Println("  • 建议先备份原文件")
	fmt.Println("  • 仅用于学习和测试目的")
}