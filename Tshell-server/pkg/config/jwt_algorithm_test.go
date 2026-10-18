package config

import (
	"reflect"
	"strings"
	"testing"
)

// jwt.algorithm 从"死配置"变成真约束的验收：
// 配置说 HS256 而实现走 RS256 时，必须**拒绝启动**而不是静默忽略。
func TestValidateJWTAlgorithm(t *testing.T) {
	// 合法：唯一支持的算法
	for _, ok := range []string{"RS256", "rs256", "Rs256"} {
		if err := ValidateJWTAlgorithm(ok); err != nil {
			t.Errorf("%q 应被接受（大小写不敏感）: %v", ok, err)
		}
	}

	// 非法：其它算法一律拒绝
	for _, bad := range []string{"HS256", "HS512", "RS512", "ES256", "none"} {
		err := ValidateJWTAlgorithm(bad)
		if err == nil {
			t.Errorf("%q 应被拒绝（当前实现只支持 RS256）", bad)
			continue
		}
		if !strings.Contains(err.Error(), bad) {
			t.Errorf("错误信息应指出是哪个取值: %v", err)
		}
	}

	// 空值也要拒：空值曾被当作"用默认"，会让实际算法变得不明
	if err := ValidateJWTAlgorithm(""); err == nil {
		t.Error("空值应被拒绝（要求显式声明，避免\"实际算法不明\"）")
	} else if !strings.Contains(err.Error(), "RS256") {
		t.Errorf("空值的错误信息应告诉运维该写什么: %v", err)
	}
}

// 默认值必须与实现一致，否则新部署一启动就失败。
// 这里直接读结构体 tag 的 default（配置加载器据它填默认值），
// 避免依赖具体的加载流程。
func TestJWTAlgorithmDefaultMatchesImplementation(t *testing.T) {
	field, ok := reflect.TypeOf(JWTConfig{}).FieldByName("Algorithm")
	if !ok {
		t.Fatal("JWTConfig 缺少 Algorithm 字段")
	}
	def := field.Tag.Get("default")
	if def == "" {
		t.Fatal("jwt.algorithm 必须有 default 标签（否则空配置会落到空值，启动即失败）")
	}
	if err := ValidateJWTAlgorithm(def); err != nil {
		t.Fatalf("默认值 %q 与实现不一致（新部署会启动失败）: %v", def, err)
	}

	// 空值必须被拒：这条同时说明"空值 = 用默认"这种假设不成立
	if err := ValidateJWTAlgorithm(""); err == nil {
		t.Error("空值应被拒绝")
	}
}
