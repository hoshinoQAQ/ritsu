package sync_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

func managed(t *testing.T) *fixture.Repo {
	t.Helper()
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)
	r.CommitAll("chore: init skeleton")
	return r
}

// SYNC-001 Tier1 上游变更强制覆盖并刷新签名。
func TestSYNC001Tier1OverwriteAndResign(t *testing.T) {
	r := managed(t)
	const hook = ".ritsu/hooks/pre-tool-use.sh"
	r.Write(hook, "# 本地乱改\n")

	res := r.Run("sync")
	fixture.AssertExit(t, res, 0)

	if strings.Contains(r.Read(hook), "本地乱改") {
		t.Fatalf("SYNC-001 Tier1 未被上游覆盖")
	}
	if !strings.Contains(r.Read(".ritsu/signatures.json"), "pre-tool-use.sh") {
		t.Fatalf("SYNC-001 签名未刷新")
	}
	fixture.AssertExit(t, r.Run("check"), 0)
}

// SYNC-002 Tier2 三向合并：本地定制与上游变更都保留。
func TestSYNC002Tier2ThreeWayMerge(t *testing.T) {
	r := managed(t)

	// 本地对 Tier2 做定制（在基线上追加）
	local := r.Read("tsconfig.json") + "\n// local-custom\n"
	r.Write("tsconfig.json", local)
	r.CommitAll("chore: local tweak")

	// 上游模板变更：模拟上游新增一行
	r.Write(".ritsu/base/tsconfig.json", r.Read(".ritsu/base/tsconfig.json")+"\n// upstream-new\n")

	res := r.Run("sync")
	// 本地改 + 上游改 = 冲突，必须要么合并要么显式报冲突
	if res.ExitCode != 0 && res.ExitCode != 1 {
		t.Fatalf("SYNC-002 退出码异常: %d", res.ExitCode)
	}
	got := r.Read("tsconfig.json")
	if res.ExitCode == 0 {
		if !strings.Contains(got, "local-custom") {
			t.Fatalf("SYNC-002 本地定制丢失: %q", got)
		}
		if !strings.Contains(got, "upstream-new") {
			t.Fatalf("SYNC-002 上游变更丢失: %q", got)
		}
		return
	}
	// 冲突路径必须写 git 风格冲突标记
	if !strings.Contains(got, "<<<<<<< local") || !strings.Contains(got, ">>>>>>> upstream") {
		t.Fatalf("SYNC-002 冲突标记格式不符: %q", got)
	}
}

// SYNC-003 Tier3 字节级不变。
func TestSYNC003Tier3Untouched(t *testing.T) {
	r := managed(t)
	const f = "src/ui/features/mine.tsx"
	r.Write(f, "export const Mine = () => null;\n")
	r.CommitAll("feat: 业务代码")
	before := r.Read(f)

	fixture.AssertExit(t, r.Run("sync"), 0)
	if r.Read(f) != before {
		t.Fatalf("SYNC-003 Tier3 被改动")
	}
}

// SYNC-004 冲突时退出 1 并给解决指引。
func TestSYNC004ConflictReportsGuidance(t *testing.T) {
	r := managed(t)
	r.Write("tsconfig.json", r.Read("tsconfig.json")+"\n// both sides\n")
	r.CommitAll("chore: local")
	r.Write(".ritsu/base/tsconfig.json", r.Read(".ritsu/base/tsconfig.json")+"\n// upstream\n")

	res := r.Run("sync")
	fixture.AssertExit(t, res, 1)
	if code := fixture.Str(t, res.Envelope, "error.code"); code != "SYNC_CONFLICT" {
		t.Fatalf("SYNC-004 error.code=%s", code)
	}
	if len(fixture.Arr(t, res.Envelope, "data.conflicts")) == 0 {
		t.Fatalf("SYNC-004 未列出冲突文件")
	}
	if len(fixture.Arr(t, res.Envelope, "next_actions")) == 0 {
		t.Fatalf("SYNC-004 未给解决指引")
	}
}

// SYNC-005 --dry-run 零写盘且给出分层 diff。
func TestSYNC005DryRunNoWrite(t *testing.T) {
	r := managed(t)
	const hook = ".ritsu/hooks/pre-tool-use.sh"
	r.Write(hook, "# 本地乱改\n")
	r.CommitAll("chore: dirty tier1")
	before := r.Snapshot()

	res := r.Run("sync", "--dry-run")
	fixture.AssertExit(t, res, 0)
	fixture.AssertNoWrite(t, r, before)

	if len(fixture.Arr(t, res.Envelope, "data.overwritten")) == 0 {
		t.Fatalf("SYNC-005 dry-run 未报告 Tier1 覆盖计划")
	}
	if fixture.Num(t, res.Envelope, "data.untouched") <= 0 {
		t.Fatalf("SYNC-005 dry-run 未统计 Tier3 不受影响项")
	}
}

// SYNC-007 缺基准版本退出 2。
func TestSYNC007NoBaselineRejected(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Dirty)
	res := r.Run("sync")
	fixture.AssertExit(t, res, 2)
	if fixture.Str(t, res.Envelope, "error.code") != "STATE_NOT_FOUND" {
		t.Fatalf("SYNC-007 error.code=%s", fixture.Str(t, res.Envelope, "error.code"))
	}
}

// SYNC-008 连跑两次幂等。
func TestSYNC008SecondRunNoChange(t *testing.T) {
	r := managed(t)
	fixture.AssertExit(t, r.Run("sync"), 0)
	mid := r.Snapshot()

	res := r.Run("sync")
	fixture.AssertExit(t, res, 0)
	after := r.Snapshot()

	for p, h := range mid {
		if after[p] != h {
			t.Fatalf("SYNC-008 第二次 sync 改动了 %s", p)
		}
	}
}
