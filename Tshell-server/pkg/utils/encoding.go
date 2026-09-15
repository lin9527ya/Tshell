package utils

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"BackendTemplate/pkg/utils/logx"
)

// ChineseEncodingProcessor 中文编码处理器
type ChineseEncodingProcessor struct {
	debug bool
}

// NewChineseEncodingProcessor 创建新的中文编码处理器
func NewChineseEncodingProcessor(debug bool) *ChineseEncodingProcessor {
	return &ChineseEncodingProcessor{
		debug: debug,
	}
}

// ProcessShellOutput 处理Shell命令输出的编码
func (c *ChineseEncodingProcessor) ProcessShellOutput(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if c.debug {
		logx.Infof("[编码处理] 原始数据长度: %d, 前50字节: %q\n", len(data), string(data[:min(50, len(data))]))
	}

	// 第一步：检查是否已经是有效的UTF-8
	if utf8.Valid(data) {
		str := string(data)
		// 检查是否包含常见的乱码模式
		if !c.containsGarbledText(str) {
			if c.debug {
				logx.Infof("[编码处理] 数据已是有效UTF-8且无乱码，直接返回\n")
			}
			return data, nil
		}
		if c.debug {
			logx.Infof("[编码处理] 数据是UTF-8但包含乱码，尝试修复\n")
		}
	}

	// 第二步：尝试不同的编码转换方法
	methods := []func([]byte) ([]byte, error){
		c.tryGBKToUTF8,
		c.tryGB18030ToUTF8,
		c.tryBig5ToUTF8,
		c.tryByteToByteGBK,
		c.tryDoubleDecoding,
	}

	for i, method := range methods {
		if result, err := method(data); err == nil && len(result) > 0 {
			if utf8.Valid(result) && !c.containsGarbledText(string(result)) {
				if c.debug {
					logx.Infof("[编码处理] 方法%d成功，结果: %q\n", i+1, string(result[:min(50, len(result))]))
				}
				return result, nil
			}
		}
	}

	// 第三步：如果所有方法都失败，返回尽可能清理的数据
	cleaned := c.cleanData(data)
	if c.debug {
		logx.Infof("[编码处理] 所有方法失败，返回清理后的数据: %q\n", string(cleaned[:min(50, len(cleaned))]))
	}
	return cleaned, nil
}

// tryGBKToUTF8 尝试GBK到UTF-8转换
func (c *ChineseEncodingProcessor) tryGBKToUTF8(data []byte) ([]byte, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("非Windows系统跳过GBK转换")
	}

	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	result, err := io.ReadAll(reader)
	return result, err
}

// tryGB18030ToUTF8 尝试GB18030到UTF-8转换
func (c *ChineseEncodingProcessor) tryGB18030ToUTF8(data []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GB18030.NewDecoder())
	result, err := io.ReadAll(reader)
	return result, err
}

// tryBig5ToUTF8 尝试Big5到UTF-8转换（台湾繁体）
// 【P2 补齐】原返回"暂未实现"错误;Big5 与 GBK 双字节区有重叠,放在 GBK/GB18030
// 尝试之后兜底,误判概率低(containsGarbledText 复核兜底)
func (c *ChineseEncodingProcessor) tryBig5ToUTF8(data []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(data), traditionalchinese.Big5.NewDecoder())
	return io.ReadAll(reader)
}

// tryByteToByteGBK 尝试字节级别的GBK转换
func (c *ChineseEncodingProcessor) tryByteToByteGBK(data []byte) ([]byte, error) {
	// 将UTF-8误认为的字符串重新按字节解释为GBK
	str := string(data)
	newBytes := make([]byte, len(str))
	for i, r := range str {
		if i < len(newBytes) {
			newBytes[i] = byte(r)
		}
	}

	reader := transform.NewReader(bytes.NewReader(newBytes), simplifiedchinese.GBK.NewDecoder())
	result, err := io.ReadAll(reader)
	return result, err
}

// tryDoubleDecoding 尝试双重解码
func (c *ChineseEncodingProcessor) tryDoubleDecoding(data []byte) ([]byte, error) {
	// 有时数据可能被双重编码，尝试双重解码
	first, err := c.tryGBKToUTF8(data)
	if err != nil {
		return nil, err
	}

	second, err := c.tryGBKToUTF8(first)
	if err != nil {
		return first, nil // 返回第一次解码的结果
	}

	return second, nil
}

// containsGarbledText 检查是否包含乱码
func (c *ChineseEncodingProcessor) containsGarbledText(str string) bool {
	// 使用十六进制转义序列表示乱码模式，避免编译错误
	garbledPatterns := []string{
		"\ufffd", // 替换字符
		"\xc3\xa4\xc2\xb8\xc2\xad\xc3\xa6\xc2\x96\xc2\x87", // 中文的常见乱码
		"\xc3\xa6\xc2\xb5\xc2\x8b\xc3\xa8\xc2\xaf\xc2\x95", // 测试的常见乱码
		"\xc3\xa4\xc2\xbd\xc2\xa0\xc3\xa5\xc2\xa5\xc2\xbd", // 你好的常见乱码
		"\xc3\xa7\xc2\xb3\xc2\xbb\xc3\xa7\xc2\xbb\xc2\x9f", // 系统的常见乱码
		"\xc3\xa6\xc2\x96\xc2\x87\xc3\xa4\xc2\xbb\xc2\xb6", // 文件的常见乱码
		"\xc3\xa7\xc2\x9b\xc2\xae\xc3\xa5\xc2\xbd\xc2\x95", // 目录的常见乱码
	}

	// 检查是否包含这些模式
	for _, pattern := range garbledPatterns {
		if strings.Contains(str, pattern) {
			return true
		}
	}

	// 检查连续的拉丁扩展字符
	latinExtendedCount := 0
	for _, r := range str {
		if r >= 0x00C0 && r <= 0x00FF {
			latinExtendedCount++
			if latinExtendedCount >= 3 {
				return true
			}
		} else {
			latinExtendedCount = 0
		}
	}

	return false
}

// cleanData 清理数据，移除不可打印字符
func (c *ChineseEncodingProcessor) cleanData(data []byte) []byte {
	var result []byte
	for _, b := range data {
		// 保留可打印的ASCII字符、换行符、制表符
		if (b >= 32 && b <= 126) || b == '\n' || b == '\r' || b == '\t' {
			result = append(result, b)
		} else if b >= 128 { // 保留高位字节（可能是中文）
			result = append(result, b)
		}
	}
	return result
}

// ProcessDatabaseContent 处理数据库中的历史内容
func (c *ChineseEncodingProcessor) ProcessDatabaseContent(content string) string {
	if content == "" {
		return content
	}

	// 检查是否需要修复
	if !c.containsGarbledText(content) {
		return content
	}

	// 尝试修复乱码
	data := []byte(content)
	fixed, err := c.ProcessShellOutput(data)
	if err != nil {
		if c.debug {
			logx.Infof("[数据库内容修复] 修复失败: %v\n", err)
		}
		return content
	}

	result := string(fixed)
	if c.debug {
		logx.Infof("[数据库内容修复] 原始: %q -> 修复: %q\n",
			content[:min(50, len(content))],
			result[:min(50, len(result))])
	}

	return result
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DefaultProcessor 默认的编码处理器实例
var DefaultProcessor = NewChineseEncodingProcessor(false)

// ProcessShellOutput 便捷函数
func ProcessShellOutput(data []byte) ([]byte, error) {
	return DefaultProcessor.ProcessShellOutput(data)
}

// ProcessDatabaseContent 便捷函数
func ProcessDatabaseContent(content string) string {
	return DefaultProcessor.ProcessDatabaseContent(content)
}
