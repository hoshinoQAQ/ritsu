package journey_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// JOURNEY-J1 新项目全链路：init 后立刻可检查、可计划、可交接。
func TestJOURNEYJ1NewProjectHappyPath(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)

	res := r.Run("init", "--stack", "ts", "--yes", "--no-interactive")
	fixture.AssertExit(t, res, 0)

	res = r.Run("check")
	fixture.AssertExit(t, res, 0)
	if n := len(fixture.Arr(t, res.Envelope, "diagnostics")); n != 0 {
		t.Fatalf("J1 新项目应为零违规，实际 %d", n)
	}

	res = r.Run("plan", "--goal", "实现用户登录")
	fixture.AssertExit(t, res, 0)

	res = r.Run("handoff", "--field", "当前进度", "--value", "骨架就绪")
	fixture.AssertExit(t, res, 0)
	if !strings.Contains(r.Read("agent/CONTINUITY.md"), "骨架就绪") {
		t.Fatalf("J1 接力卡未更新")
	}
}

// JOURNEY-J2 存量纳管全链路：存量放行、新增零容忍。
func TestJOURNEYJ2AdoptLegacyThenRatchet(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)

	fixture.AssertExit(t, r.Run("adopt", "--stack", "ts", "--yes"), 0)
	fixture.AssertExit(t, r.Run("check"), 0)

	// 新增一条与存量同规则的违规，必须只报新增
	r.Write("src/ui/features/another.css", ".b{color:#abcdef}\n")
	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if f, _ := m["file"].(string); f != "src/ui/features/another.css" {
			t.Fatalf("J2 棘轮泄漏存量违规: %s", f)
		}
	}
}

// JOURNEY-J5 红绿证据链 → 完成计划 的收尾链路。
func TestJOURNEYJ5EvidenceThenComplete(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--stack", "ts", "--yes", "--no-interactive"), 0)

	res := r.Run("plan", "--goal", "接入支付")
	fixture.AssertExit(t, res, 0)
	created := fixture.Str(t, res.Envelope, "data.created")

	// 没有证据链时必须红
	res = r.Run("verify")
	fixture.AssertExit(t, res, 1)

	r.Write(".ritsu/logs/red.txt", "--- FAIL: TestPay\n")
	r.Write(".ritsu/logs/green.txt", "--- PASS: TestPay\n")
	r.Write(".ritsu/redgreen.json", string(fixture.Encode(t, map[string]any{
		"intent":        "接入支付",
		"scope":         "src",
		"plan":          created,
		"changes":       []string{"src/service/pay.ts"},
		"red_command":   "go test ./pay/...",
		"red_log":       ".ritsu/logs/red.txt",
		"red_time":      "2026-01-01T00:00:00Z",
		"green_command": "go test ./pay/...",
		"green_log":     ".ritsu/logs/green.txt",
		"green_time":    "2026-01-01T02:00:00Z",
		"side_effects":  []string{},
		"warnings":      []string{},
	})))

	res = r.Run("verify")
	fixture.AssertExit(t, res, 0)
	if !r.Exists(".ritsu/artifact.json") {
		t.Fatalf("J5 未产出证据包")
	}

	name := created[strings.LastIndex(created, "/")+1:]
	res = r.Run("plan", "--complete", name)
	fixture.AssertExit(t, res, 0)
	if r.Exists(created) {
		t.Fatalf("J5 active 计划未移除")
	}
	if !r.Exists("docs/exec-plans/completed/" + name) {
		t.Fatalf("J5 计划未归档")
	}
}
