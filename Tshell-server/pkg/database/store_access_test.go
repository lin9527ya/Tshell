package database

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestStoreAccessRatchet 棘轮测试：业务层不得重新直接拼 SQL（阶段三 §3.7）。
//
// 为什么用"棘轮"而不是"一刀切禁止"：
//
//	收口是分批做的，过程中必然存在"已迁移 + 未迁移"并存。棘轮记录**当前上限**，
//	只许降不许升 —— 既不会让未迁移的部分卡住 CI，又能保证新写的代码一律走领域层，
//	且每次迁移完顺手把上限调小，进度只进不退。
//
// 允许保留的两类（不算违规）：
//   - `database.DB() == nil` / `!= nil`：引擎就绪判定，它本来就是"问有没有引擎"，
//     不是查询；换成领域方法反而绕远。
//   - `database.DB().NewSession()`：需要跨多条语句的事务/复合操作，
//     领域层按单次查询设计，硬塞进来只会让方法签名变成万能参数。
//
// 若要放宽上限，请在同一个提交里说明理由 —— 这个测试的存在意义就是让"放宽"
// 变成一个有意识的动作，而不是顺手。
func TestStoreAccessRatchet(t *testing.T) {
	// 上限 0：业务代码里**不允许**再出现直接查库。
	//
	// 只统计非 _test.go 文件：测试为了造数据/断言需要直接读库，那是测试的正当手段，
	// 强行套领域层只会让测试绕远路。
	const maxDirectCalls = 0

	apiDir := filepath.Join("..", "api")
	entries, err := os.ReadDir(apiDir)
	if err != nil {
		t.Skipf("跳过：读不到 pkg/api（%v）", err)
	}

	// 只统计"查询型"调用：排除 nil 判定与 NewSession
	guardRe := regexp.MustCompile(`database\.DB\(\)\s*(==|!=)\s*nil`)
	sessionRe := regexp.MustCompile(`database\.DB\(\)\s*\.\s*NewSession\(\)`)
	callRe := regexp.MustCompile(`database\.DB\(\)`)

	total := 0
	var offenders []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(apiDir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", path, err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if !callRe.MatchString(line) {
				continue
			}
			if guardRe.MatchString(line) || sessionRe.MatchString(line) {
				continue
			}
			total++
			offenders = append(offenders, strings.TrimSpace(
				filepath.Base(path)+":"+itoa(i+1)+"  "+strings.TrimSpace(line)))
		}
	}

	if total > maxDirectCalls {
		t.Errorf("业务层直接查库的调用数 %d 超过棘轮上限 %d —— 新代码请用领域方法"+
			"（database.Listeners()/Clients_()/Deliveries() 等，见 pkg/database/domain_*.go）:\n  %s",
			total, maxDirectCalls, strings.Join(offenders, "\n  "))
	}
	t.Logf("业务层直接查库调用数 = %d（棘轮上限 %d）", total, maxDirectCalls)
}

// TestDomainMethodsCoverCoreEntities 守卫领域层的关键实体入口仍然存在。
//
// 棘轮只管"数量不涨"，管不了"方法被误删/改签名"。这里按名字钉住几个
// 业务层最依赖的入口，签名改动会在编译期先炸掉 —— 这条测的是"入口还在"。
func TestDomainMethodsCoverCoreEntities(t *testing.T) {
	// 编译期即验证：这些调用必须能通过类型检查
	var (
		_ = Listeners()
		_ = Clients_()
		_ = NotesStore_()
		_ = DownloadsStore_()
		_ = Shells()
		_ = LootStore_()
		_ = Deliveries()
		_ = DeliveryLogs()
		_ = Tunnels()
		_ = Dashboard()
		_ = Tasks()
		_ = Users_()
		_ = Plugins()
		_ = StagerTokens()
	)
}

// TestErrStoreUnavailableIsSentinel 领域方法在引擎未初始化时返回哨兵错误，
// 业务层才能用 errors.Is 区分"环境问题"与"数据问题"。
func TestErrStoreUnavailableIsSentinel(t *testing.T) {
	if ErrStoreUnavailable == nil {
		t.Fatal("ErrStoreUnavailable 不应为 nil")
	}
	// 引擎未初始化时（测试进程默认如此）领域方法应返回它而不是 panic
	old := Engine
	Engine = nil
	defer func() { Engine = old }()

	if _, err := Listeners().List(); err == nil {
		t.Error("引擎未初始化时 List 应返回错误，而不是静默返回空列表")
	} else if !strings.Contains(err.Error(), "未初始化") {
		t.Errorf("错误信息应说明原因，实际: %v", err)
	}
	if _, _, err := Clients_().GetByUID("x"); err == nil {
		t.Error("引擎未初始化时 GetByUID 应返回错误")
	}
}

// itoa 避免为一个数字格式化引入 strconv
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
