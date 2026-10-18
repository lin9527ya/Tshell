package main

import (
	"os"
	"os/exec"
	"testing"
)

// TestFileModeBuildWorks 守住权威构建入口的形态。
//
// build.sh 用**文件模式** `go build ./main.go`（Windows+TDM-GCC 下验证过的唯一
// 可用形态），而文件模式只编译被点名的那个文件 —— main 包里一旦多出第二个
// 非测试 .go（把 CLI 实现直接写在根目录就会这样），构建立刻失败：
// 实测 `./main.go:123:11: undefined: runListBackups`。
//
// CI 跑的是包模式 `go build ./...`，这类破坏在 CI 上是绿的，只有真的执行
// build.sh 才会炸。因此这条用例直接按文件模式编译一次。
func TestFileModeBuildWorks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("没有 go 工具链，跳过")
	}
	out := os.DevNull
	cmd := exec.Command("go", "build", "-o", out, "./main.go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("文件模式构建失败（build.sh 会因此不可用）: %v\n%s", err, b)
	}
}
