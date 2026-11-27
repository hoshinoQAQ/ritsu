package check_test

import (
	"fmt"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

func managed(t *testing.T) *fixture.Repo {
	t.Helper()
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)
	// 新生成骨架必须自身全绿，否则后续用例的基线不成立
	fixture.AssertExit(t, r.Run("check"), 0)
	return r
}

func diagsOf(t *testing.T, res fixture.Result) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		if m, ok := d.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func firstRule(t *testing.T, res fixture.Result) string {
	t.Helper()
	ds := diagsOf(t, res)
	if len(ds) == 0 {
		t.Fatalf("期望至少一条诊断: %s", res.Stdout)
	}
	s, _ := ds[0]["rule_id"].(string)
	return s
}

// CHECK-001 分层反向 import。
func TestCHECK001LayerDirection(t *testing.T) {
	r := managed(t)
	// service 层反向 import ui 层
	r.Write("src/service/leak.ts", "import { Button } from '../ui/primitives/Button';\nexport const x = Button;\n")

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	ds := diagsOf(t, res)
	if len(ds) == 0 {
		t.Fatalf("CHECK-001 未报告反向依赖")
	}
	d := ds[0]
	if d["rule_id"] != "layer-direction" {
		t.Fatalf("CHECK-001 规则 id=%v", d["rule_id"])
	}
	if s, _ := d["file"].(string); s == "" {
		t.Fatalf("CHECK-001 诊断缺文件")
	}
	if n, _ := d["line"].(float64); n <= 0 {
		t.Fatalf("CHECK-001 诊断缺行号")
	}
}

// CHECK-002 裸 SQL 出现在非 db 目录。
func TestCHECK002SQLOutsideDB(t *testing.T) {
	r := managed(t)
	r.Write("src/service/repo.ts", "export const all = 'SELECT id FROM users';\n")

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)
	if got := firstRule(t, res); got != "sql-outside-db" {
		t.Fatalf("CHECK-002 规则 id=%s", got)
	}
}

// CHECK-003 Tailwind 任意色值。
func TestCHECK003ArbitraryColor(t *testing.T) {
	r := managed(t)
	r.Write("src/ui/features/badge.tsx", "export const B = () => <span className=\"bg-[#330745]\" />;\n")

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)
	if got := firstRule(t, res); got != "no-arbitrary-color" {
		t.Fatalf("CHECK-003 规则 id=%s", got)
	}
}

// CHECK-004 业务 UI 目录禁用原生 HTML；primitives 内不报。
func TestCHECK004NativeHTMLOnlyOutsidePrimitives(t *testing.T) {
	r := managed(t)
	r.Write("src/ui/features/bad.tsx", "export const Bad = () => <button>ok</button>;\n")

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)
	if got := firstRule(t, res); got != "no-native-html" {
		t.Fatalf("CHECK-004 正例规则 id=%s", got)
	}

	// 反例：同样的写法放进 primitives 是合法基础组件
	r.Write("src/ui/features/bad.tsx", "export const ok = 1;\n")
	r.Write("src/ui/primitives/Raw.tsx", "export const Raw = () => <button>ok</button>;\n")
	fixture.AssertExit(t, r.Run("check"), 0)
}

// CHECK-008 --scope 限定目录。
func TestCHECK008ScopeLimitsSearch(t *testing.T) {
	r := managed(t)
	r.Write("src/service/in.ts", "export const q = 'SELECT 1 FROM t';\n")
	r.Write("src/db/out.ts", "export const q = 'SELECT 1 FROM t';\n")

	res := r.Run("check", "--scope", "src/db")
	fixture.AssertExit(t, res, 0)
	if n := len(diagsOf(t, res)); n != 0 {
		t.Fatalf("CHECK-008 目录外违规泄漏了 %d 条", n)
	}

	res = r.Run("check", "--scope", "src/service")
	fixture.AssertExit(t, res, 1)
}

// CHECK-009 --strict 把 warning 升级为 error。
func TestCHECK009StrictUpgradesWarnings(t *testing.T) {
	r := managed(t)
	r.Write("src/service/log.ts", "export function f() { console.log('x'); }\n")

	fixture.AssertExit(t, r.Run("check"), 0) // 非 strict 下 warning 被丢弃
	res := r.Run("check", "--strict")
	fixture.AssertExit(t, res, 1)

	var found bool
	for _, d := range diagsOf(t, res) {
		if d["rule_id"] == "no-console-log" {
			found = true
			if d["severity"] != "error" {
				t.Fatalf("CHECK-009 告警未升级: %v", d["severity"])
			}
		}
	}
	if !found {
		t.Fatalf("CHECK-009 strict 下未报告 no-console-log")
	}
}

// CHECK-011 命中 0 文件必须报错，不能空集假绿。
func TestCHECK011EmptyScopeFails(t *testing.T) {
	r := managed(t)
	r.Mkdir("contracts/empty-dir")

	res := r.Run("check", "--scope", "contracts/empty-dir")
	fixture.AssertExit(t, res, 1)
	if code := fixture.Str(t, res.Envelope, "error.code"); code == "" {
		t.Fatalf("CHECK-011 失败必须带 error.code")
	}
	if len(fixture.Arr(t, res.Envelope, "next_actions")) == 0 {
		t.Fatalf("CHECK-011 应给出可执行的下一步")
	}
}

// CHECK-012 依赖图边数低于 min_edges。
func TestCHECK012MinEdgesSelfProof(t *testing.T) {
	r := managed(t)
	arch := r.Read(".ritsu/architecture.yaml")
	r.Write(".ritsu/architecture.yaml", bumpMinEdges(arch, 9999))

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	var found bool
	for _, d := range diagsOf(t, res) {
		if d["rule_id"] == "graph-selfproof" {
			found = true
		}
	}
	if !found {
		t.Fatalf("CHECK-012 未报告 graph-selfproof: %s", res.Stdout)
	}
}

// CHECK-014 --fields 裁剪输出。
func TestCHECK014FieldsProjection(t *testing.T) {
	r := managed(t)
	res := r.Run("check", "--fields", "status,command")
	fixture.AssertExit(t, res, 0)

	env := fixture.ParseJSON(t, res.Stdout)
	for _, k := range []string{"status", "command", "schemaVersion"} {
		if _, ok := env[k]; !ok {
			t.Fatalf("CHECK-014 缺少保留字段 %s", k)
		}
	}
	// schemaVersion 是协议必需项，裁剪后仍保留；其余未指定键必须消失
	for _, k := range []string{"diagnostics", "next_actions", "data"} {
		if _, ok := env[k]; ok {
			t.Fatalf("CHECK-014 未裁剪字段 %s", k)
		}
	}
}

// CHECK-015 无违规时 diagnostics 必须是空数组不是 null。
func TestCHECK015CleanRepoEmptyArray(t *testing.T) {
	r := managed(t)
	res := r.Run("check")
	fixture.AssertExit(t, res, 0)

	if !strings.Contains(res.Stdout, `"diagnostics": []`) {
		t.Fatalf("CHECK-015 期望空数组: %s", res.Stdout)
	}
	if v, ok := fixture.ParseJSON(t, res.Stdout)["diagnostics"]; !ok || v == nil {
		t.Fatalf("CHECK-015 diagnostics 为 null")
	}
}

// bumpMinEdges 把 graph.min_edges 改写为指定值。
func bumpMinEdges(doc string, n int) string {
	lines := strings.Split(doc, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "min_edges:") {
			indent := ln[:len(ln)-len(strings.TrimLeft(ln, " \t"))]
			lines[i] = fmt.Sprintf("%smin_edges: %d", indent, n)
		}
	}
	return strings.Join(lines, "\n")
}
