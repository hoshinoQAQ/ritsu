package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// CHECK-005 多引擎报同一违规时只保留一条。
func TestCHECK005MultiEngineDedupe(t *testing.T) {
	r := managed(t)
	const body = `console.log(JSON.stringify([{rule_id:"dup-rule",severity:"error",file:"src/service/a.ts",line:1,message:"同一违规"}]));`
	r.Write("tools/e1.mjs", body)
	r.Write("tools/e2.mjs", body)
	r.Write(".ritsu/engines.json", `{"engines":[
		{"id":"e1","command":"node","args":["tools/e1.mjs"],"timeout_ms":10000},
		{"id":"e2","command":"node","args":["tools/e2.mjs"],"timeout_ms":10000}]}`)

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	count := 0
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if m["rule_id"] == "dup-rule" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("CHECK-005 期望去重为 1 条，实际 %d 条", count)
	}
}

// CHECK-006 --format agent 的紧凑输出保留 flow_steps。
func TestCHECK006AgentFormatKeepsFlowSteps(t *testing.T) {
	r := managed(t)
	// 反向依赖天然带成因链
	r.Write("src/service/leak.ts", "import { Button } from '../ui/primitives/Button';\nexport const x = Button;\n")

	res := r.Run("check", "--format", "agent")
	fixture.AssertExit(t, res, 1)

	compact := fixture.Arr(t, res.Envelope, "data.compact")
	if len(compact) == 0 {
		t.Fatalf("CHECK-006 compact 为空")
	}
	first, _ := compact[0].(map[string]any)
	steps, _ := first["flow_steps"].([]any)
	if len(steps) < 2 {
		t.Fatalf("CHECK-006 flow_steps 不完整: %v", first)
	}
}

// CHECK-007 --format sarif 输出必须符合 SARIF 2.1.0 必填结构。
func TestCHECK007SarifSchema(t *testing.T) {
	r := managed(t)
	r.Write("src/service/q.ts", "export const q = 'SELECT a FROM t';\n")

	res := r.Run("check", "--format", "sarif")
	fixture.AssertExit(t, res, 1)

	sarif, _ := fixture.ParseJSON(t, res.Stdout)["data"].(map[string]any)["sarif"].(map[string]any)
	if sarif["version"] != "2.1.0" {
		t.Fatalf("CHECK-007 version=%v", sarif["version"])
	}
	runs, _ := sarif["runs"].([]any)
	if len(runs) == 0 {
		t.Fatalf("CHECK-007 runs 为空")
	}
	run, _ := runs[0].(map[string]any)
	tool, _ := run["tool"].(map[string]any)
	driver, _ := tool["driver"].(map[string]any)
	if driver["name"] != "ritsu" {
		t.Fatalf("CHECK-007 tool.driver.name=%v", driver["name"])
	}
	results, _ := run["results"].([]any)
	if len(results) == 0 {
		t.Fatalf("CHECK-007 results 为空")
	}
	first, _ := results[0].(map[string]any)
	if first["ruleId"] == nil || first["message"] == nil || first["locations"] == nil {
		t.Fatalf("CHECK-007 结果缺必填字段: %v", first)
	}
}

// CHECK-010 引擎二进制缺失时必须说明缺哪个并给安装指引，不是崩溃栈。
func TestCHECK010EngineMissing(t *testing.T) {
	r := managed(t)
	r.Write(".ritsu/engines.json",
		`{"engines":[{"id":"ghost","command":"no-such-engine-xyz-42","args":[],"timeout_ms":2000}]}`)

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	if code := fixture.Str(t, res.Envelope, "error.code"); code != "ENGINE_MISSING" {
		t.Fatalf("CHECK-010 error.code=%s", code)
	}
	msg := fixture.Str(t, res.Envelope, "error.message") + fixture.Str(t, res.Envelope, "error.hint")
	if !strings.Contains(msg, "ghost") {
		t.Fatalf("CHECK-010 未指明缺哪个引擎: %s", msg)
	}
	if strings.Contains(res.Stderr, "goroutine") {
		t.Fatalf("CHECK-010 出现崩溃栈")
	}
}

// CHECK-013 超大输出必须截断并给出可读的 full_log_path。
func TestCHECK013TruncationWithReadableLog(t *testing.T) {
	r := managed(t)
	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString("export const q" + itoa(i) + " = 'SELECT a FROM t';\n")
	}
	r.Write("src/service/many.ts", b.String())

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	if n := len(fixture.Arr(t, res.Envelope, "diagnostics")); n > 30 {
		t.Fatalf("CHECK-013 诊断未截断: %d 条", n)
	}
	if !fixture.Bool(t, res.Envelope, "truncated") {
		t.Fatalf("CHECK-013 缺 truncated 标记")
	}
	full := fixture.Str(t, res.Envelope, "full_log_path")
	if full == "" {
		t.Fatalf("CHECK-013 缺 full_log_path")
	}
	if !filepath.IsAbs(full) {
		t.Fatalf("CHECK-013 full_log_path 必须是绝对路径: %s", full)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("CHECK-013 指针文件不可读: %v", err)
	}
	if n := strings.Count(string(data), "rule_id"); n < 30 {
		t.Fatalf("CHECK-013 全量日志条目过少: %d", n)
	}
}

// CHECK-016 两个进程并发 check 互不干扰。
func TestCHECK016ConcurrentChecks(t *testing.T) {
	r := managed(t)
	r.Write("src/service/q.ts", "export const q = 'SELECT a FROM t';\n")

	var wg sync.WaitGroup
	results := make([]fixture.Result, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = r.Run("check")
		}(i)
	}
	wg.Wait()

	for i, res := range results {
		if res.ExitCode != 1 {
			t.Fatalf("CHECK-016 第 %d 个进程退出码=%d", i, res.ExitCode)
		}
		if n := len(fixture.Arr(t, res.Envelope, "diagnostics")); n == 0 {
			t.Fatalf("CHECK-016 第 %d 个进程没有结果", i)
		}
	}
	if temps := r.StrayTemps(); len(temps) != 0 {
		t.Fatalf("CHECK-016 并发残留临时文件: %v", temps)
	}
}

// CHECK-017 规则被删但样本仍在：孤儿样本必须报错。
func TestCHECK017OrphanRuleTest(t *testing.T) {
	r := managed(t)
	r.Write(".ritsu/rule-tests/ghost-rule.json",
		`{"file":"src/ui/features/a.tsx","snippet":"<button/>","expect":1}`)

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	var hit bool
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if m["rule_id"] == "rule-test-orphan" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("CHECK-017 未报告孤儿样本: %s", res.Stdout)
	}
}

// CHECK-018 样本存在但规则实际不报错：规则自证必须红。
func TestCHECK018RuleSelfProofFails(t *testing.T) {
	r := managed(t)
	// 样本声明必然命中 1 次，但内容不含原生 HTML，规则不会命中
	r.Write(".ritsu/rule-tests/no-native-html.json",
		`{"file":"src/ui/features/ok.tsx","snippet":"export const ok = 1;","expect":1}`)

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	var hit bool
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if m["rule_id"] == "rule-selfproof" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("CHECK-018 未报告规则自证失败: %s", res.Stdout)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
