package fault_test

import (
	"strings"
	"testing"
	"time"

	"ritsu/tests/e2e/fixture"
)

func managed(t *testing.T) *fixture.Repo {
	t.Helper()
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)
	r.CommitAll("chore: skeleton")
	return r
}

// FAULT-001 写盘失败（目标父级被文件占位）必须退出 1，且不留下半截文件。
func TestFAULT001WriteFailureNoPartialFile(t *testing.T) {
	r := managed(t)
	// 把 docs/exec-plans/active 变成普通文件，后续写计划必然失败
	r.RemovePath("docs/exec-plans/active")
	r.Write("docs/exec-plans/active", "占位文件，不是目录\n")
	before := r.Snapshot()

	res := r.Run("plan", "--goal", "写盘失败场景")
	fixture.AssertExit(t, res, 1)
	if code := fixture.Str(t, res.Envelope, "error.code"); code == "" {
		t.Fatalf("FAULT-001 失败必须带 error.code")
	}
	if strings.Contains(res.Stdout, "panic") || strings.Contains(res.Stderr, "panic") {
		t.Fatalf("FAULT-001 不得 panic: %s", res.Stderr)
	}
	fixture.AssertNoWrite(t, r, before)
	if temps := r.StrayTemps(); len(temps) != 0 {
		t.Fatalf("FAULT-001 残留临时文件: %v", temps)
	}
}

// FAULT-002 目标不可写时退出 1 并指明路径。
func TestFAULT002PermissionDeniedNamesPath(t *testing.T) {
	r := managed(t)
	const target = ".ritsu/hooks/pre-tool-use.sh"
	// 本地改动后设为只读，sync 覆盖时会失败
	r.Write(target, "# 本地改动\n")
	r.ReadOnly(target)

	res := r.Run("sync")
	fixture.AssertExit(t, res, 1)

	msg := fixture.Str(t, res.Envelope, "error.message") + fixture.Str(t, res.Envelope, "error.path")
	if !strings.Contains(msg, "pre-tool-use") {
		t.Fatalf("FAULT-002 未指明失败路径: %s", res.Stdout)
	}
}

// FAULT-003 写盘途中被强杀：要么旧内容要么新内容，重启后无临时文件残留。
func TestFAULT003KillMidWriteIsAtomic(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	// 在 60ms 后强杀，此时 fsx 已在写临时文件但尚未 rename
	_, _ = r.Kill(60, "init", "--stack", "ts", "--yes", "--no-interactive")
	// 临时文件需超过清理阈值（3s）才算"上次运行的残留"
	time.Sleep(4 * time.Second)

	// 强杀后仓库里允许有临时文件，但下一次任意命令启动必须清干净
	fixture.AssertExit(t, r.Run("version"), 0)
	if temps := r.StrayTemps(); len(temps) != 0 {
		t.Fatalf("FAULT-003 重启后仍残留临时文件: %v", temps)
	}

	// 已落盘的文件要么不存在要么内容完整，不能是半截
	if r.Exists("AGENTS.md") {
		if !strings.HasSuffix(r.Read("AGENTS.md"), "\n") {
			t.Fatalf("FAULT-003 落盘文件被截断")
		}
	}

	// 重跑必须幂等成功
	fixture.AssertExit(t, r.Run("init", "--stack", "ts", "--yes", "--no-interactive"), 0)
}

// FAULT-005 git 索引被占用时必须明确报错，不得静默成功。
func TestFAULT005GitLockConflictReported(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	r.Write(".git/index.lock", "")

	res := r.Run("adopt", "--stack", "ts", "--yes")
	fixture.AssertExit(t, res, 1)

	msg := fixture.Str(t, res.Envelope, "error.message")
	if !strings.Contains(msg, "index.lock") {
		t.Fatalf("FAULT-005 未说明索引被占用: %s", res.Stdout)
	}
	if len(fixture.Arr(t, res.Envelope, "next_actions")) == 0 && fixture.Str(t, res.Envelope, "error.hint") == "" {
		t.Fatalf("FAULT-005 应给出清理指引")
	}
}

// FAULT-006 仓库内的符号链接不得逃逸出作用域。
func TestFAULT006SymlinkEscapeRejected(t *testing.T) {
	r := managed(t)
	outside := t.TempDir()
	r.Symlink(outside, "src/link-out")

	res := r.Run("check", "--scope", "src/link-out")
	fixture.AssertExit(t, res, 2)
	if code := fixture.Str(t, res.Envelope, "error.code"); code != "USAGE_SCOPE_OUTSIDE" {
		t.Fatalf("FAULT-006 error.code=%s", code)
	}
}

// FAULT-008 运行中配置被改：本次结果必须基于启动时快照，不半新半旧。
func TestFAULT008ConfigChangedMidRun(t *testing.T) {
	r := managed(t)
	// 造一条违规，判定结果基于「启动时」的配置
	r.Write("src/ui/features/x.css", ".a{color:#123456}\n")

	archPath := ".ritsu/architecture.yaml"
	res := r.RunDuring(220, 400, func() {
		// 窗口内删掉配置：若实现是边跑边读，就会崩或半新半旧
		r.RemovePath(archPath)
	}, "check")

	fixture.AssertExit(t, res, 1)
	if strings.Contains(res.Stderr, "panic") {
		t.Fatalf("FAULT-008 配置中途消失导致崩溃: %s", res.Stderr)
	}
	// 基于快照：仍然报出既有规则违规，而不是变成「缺配置」
	if code := fixture.Str(t, res.Envelope, "error.code"); code != "CHECK_VIOLATIONS" {
		t.Fatalf("FAULT-008 结果不确定，error.code=%s", code)
	}
}
