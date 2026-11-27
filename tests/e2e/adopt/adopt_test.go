package adopt_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// ADOPT-001 纳管 legacy-ts 生成 baseline。
func TestADOPT001AdoptsLegacy(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	if s := r.GitOut("status", "--porcelain"); s != "" {
		t.Fatalf("夹具仓库不干净: %q dir=%s", s, r.Dir)
	}
	res := r.Run("adopt", "--stack", "ts", "--yes")
	fixture.AssertExit(t, res, 0)

	if fixture.Num(t, res.Envelope, "data.baseline") <= 0 {
		t.Fatalf("ADOPT-001 baseline 条目数应为正: %s", res.Stdout)
	}
	if !r.Exists(".ritsu/ratchet-baseline.json") {
		t.Fatalf("ADOPT-001 未写棘轮账本")
	}
}

// ADOPT-002 每条 baseline 必须带 reason。
func TestADOPT002EveryEntryHasReason(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	fixture.AssertExit(t, r.Run("adopt", "--stack", "ts", "--yes"), 0)

	var env map[string]any
	body := r.Read(".ritsu/ratchet-baseline.json")
	env = fixture.ParseJSON(t, body)
	entries, _ := env["entries"].([]any)
	if len(entries) == 0 {
		t.Fatalf("ADOPT-002 账本为空")
	}
	for i, e := range entries {
		m, _ := e.(map[string]any)
		if s, _ := m["reason"].(string); strings.TrimSpace(s) == "" {
			t.Fatalf("ADOPT-002 第 %d 条缺 reason: %v", i, m)
		}
	}
}

// ADOPT-003 纳管后 check 全绿（存量被放行）。
func TestADOPT003CheckGreenAfterAdopt(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	fixture.AssertExit(t, r.Run("adopt", "--stack", "ts", "--yes"), 0)

	res := r.Run("check")
	fixture.AssertExit(t, res, 0)
	if n := len(fixture.Arr(t, res.Envelope, "diagnostics")); n != 0 {
		t.Fatalf("ADOPT-003 纳管后仍有 %d 条诊断", n)
	}
}

// ADOPT-004 纳管后新增违规必须只指向新增。
func TestADOPT004RatchetOnlyReportsNew(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	fixture.AssertExit(t, r.Run("adopt", "--stack", "ts", "--yes"), 0)

	r.Write("src/ui/features/extra.css", ".b{color:#111111}\n")

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)
	diags := fixture.Arr(t, res.Envelope, "diagnostics")
	if len(diags) == 0 {
		t.Fatalf("ADOPT-004 新增违规未被报告")
	}
	for _, d := range diags {
		m, _ := d.(map[string]any)
		if f, _ := m["file"].(string); f != "src/ui/features/extra.css" {
			t.Fatalf("ADOPT-004 棘轮泄漏了存量违规: %s", f)
		}
	}
}

// ADOPT-005 不覆盖既有源码。
func TestADOPT005NeverOverwritesSources(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	before := r.Snapshot()
	fixture.AssertExit(t, r.Run("adopt", "--stack", "ts", "--yes"), 0)
	after := r.Snapshot()

	for p, h := range before {
		if got, ok := after[p]; !ok || got != h {
			t.Fatalf("ADOPT-005 覆盖了既有文件: %s", p)
		}
	}
}

// ADOPT-006 工作树不干净拒绝纳管。
func TestADOPT006DirtyTreeRejected(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	r.Write("src/service/user.ts", "export const get = (id: any) => id; // dirty\n")

	res := r.Run("adopt", "--stack", "ts", "--yes")
	fixture.AssertExit(t, res, 2)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.hint"), "stash") &&
		!strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "未提交") {
		t.Fatalf("ADOPT-006 应说明如何清理工作树: %s", res.Stdout)
	}
}

// ADOPT-007 已纳管仓库拒绝重复纳管。
func TestADOPT007AlreadyManagedRejected(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	res := r.Run("adopt", "--stack", "ts", "--yes")
	fixture.AssertExit(t, res, 2)
	if !strings.Contains(strings.ToLower(fixture.Str(t, res.Envelope, "error.hint")), "sync") {
		t.Fatalf("ADOPT-007 应引导使用 sync: %s", res.Stdout)
	}
}

// ADOPT-009 baseline 指纹不受行号影响。
func TestADOPT009FingerprintStableOnLineShift(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	fixture.AssertExit(t, r.Run("adopt", "--stack", "ts", "--yes"), 0)
	before := fixture.Num(t, r.Run("check").Envelope, "data.violations")

	r.Write("src/db/store.ts", "// shift\n"+r.Read("src/db/store.ts"))
	res := r.Run("check")
	fixture.AssertExit(t, res, 0)
	if got := fixture.Num(t, res.Envelope, "data.violations"); got != before {
		t.Fatalf("ADOPT-009 行号变化导致指纹漂移: %v -> %v", before, got)
	}
}

// ADOPT-010 账本被删条目需显式确认。
func TestADOPT010ShrunkBaselineRequiresAck(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	fixture.AssertExit(t, r.Run("adopt", "--stack", "ts", "--yes"), 0)

	body := r.Read(".ritsu/ratchet-baseline.json")
	env := fixture.ParseJSON(t, body)
	entries, _ := env["entries"].([]any)
	if len(entries) == 0 {
		t.Skip("账本为空，无法构造缩表场景")
	}
	env["entries"] = entries[1:]
	r.Write(".ritsu/ratchet-baseline.json", string(fixture.Encode(t, env)))

	fixture.AssertExit(t, r.Run("check"), 1)
	fixture.AssertExit(t, r.Run("check", "--accept-debt"), 0)
}
