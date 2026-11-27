package init_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

const yes = "--yes"

func initArgs(extra ...string) []string {
	return append([]string{"init", "--no-interactive", yes}, extra...)
}

// INIT-001 空仓库初始化生成 Tier1/2/3 全套。
func TestINIT001GeneratesFullSkeleton(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run(initArgs("--stack", "ts")...)
	fixture.AssertExit(t, res, 0)

	for _, p := range []string{
		"AGENTS.md",
		".ritsu/architecture.yaml",
		".ritsu/signatures.json",
		"CODEOWNERS",
		"docs/architecture/layers.md",
		"agent/CONTINUITY.md",
		"src/ui/primitives/Button.tsx",
		"src/types/index.ts",
		"src/db/.gitkeep",
	} {
		if !r.Exists(p) {
			t.Fatalf("INIT-001 缺少产物: %s", p)
		}
	}
	// Tier1 只读层带标记
	if !strings.Contains(r.Read(".ritsu/hooks/pre-tool-use.sh"), "read-only: managed by ritsu") {
		t.Fatalf("INIT-001 Tier1 文件缺少只读标记")
	}
	if !strings.Contains(r.Read("CODEOWNERS"), "src/ui") {
		t.Fatalf("INIT-001 CODEOWNERS 未覆盖 Tier3 路径")
	}
}

// INIT-002 AGENTS.md 行数落在 60-100。
func TestINIT002AgentsLineBudget(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run(initArgs("--stack", "ts")...), 0)

	lines := len(strings.Split(strings.TrimRight(r.Read("AGENTS.md"), "\n"), "\n"))
	if lines < 60 || lines > 100 {
		t.Fatalf("INIT-002 AGENTS.md 行数 %d 越界（期望 60-100）", lines)
	}
}

// INIT-003 AGENTS.md 只放链接与红线，不复制正本正文。
func TestINIT003AgentsHasNoDocBody(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run(initArgs("--stack", "ts")...), 0)

	agents := r.Read("AGENTS.md")
	for _, ln := range strings.Split(r.Read("docs/architecture/layers.md"), "\n") {
		ln = strings.TrimSpace(ln)
		if len([]rune(ln)) < 24 || strings.HasPrefix(ln, "#") {
			continue
		}
		if strings.Contains(agents, ln) {
			t.Fatalf("INIT-003 AGENTS.md 复制了正本正文: %q", ln)
		}
	}
}

// INIT-004 Tier1 可签名校验，手改一字节必红。
func TestINIT004Tier1SignatureTamperDetected(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run(initArgs("--stack", "ts")...), 0)

	const tier1 = ".ritsu/hooks/pre-tool-use.sh"
	if !strings.Contains(r.Read(".ritsu/signatures.json"), "pre-tool-use.sh") {
		t.Fatalf("INIT-004 签名清单未覆盖 Tier1 文件")
	}
	r.Write(tier1, r.Read(tier1)+"# tampered\n")

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if s, _ := m["file"].(string); s == tier1 {
			return
		}
	}
	t.Fatalf("INIT-004 篡改未被 check 捕获: %s", res.Stdout)
}

// INIT-005 CONTINUITY.md 恰好八个字段。
func TestINIT005ContinuityEightFields(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run(initArgs("--stack", "ts")...), 0)

	var heads []string
	for _, ln := range strings.Split(r.Read("agent/CONTINUITY.md"), "\n") {
		if strings.HasPrefix(ln, "## ") {
			heads = append(heads, strings.TrimSpace(strings.TrimPrefix(ln, "## ")))
		}
	}
	if len(heads) != 8 {
		t.Fatalf("INIT-005 CONTINUITY 小节数 %d，期望 8: %v", len(heads), heads)
	}
}

// INIT-006 纯 TS 走 Turborepo。
func TestINIT006PureTSRoutesTurbo(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run(initArgs("--stack", "ts")...)
	fixture.AssertExit(t, res, 0)

	if got := fixture.Str(t, res.Envelope, "data.orchestrator"); got != "turborepo" {
		t.Fatalf("INIT-006 编排器=%s，期望 turborepo", got)
	}
	if !r.Exists("turbo.json") || r.Exists("moon.yml") || r.Exists("nx.json") {
		t.Fatalf("INIT-006 编排器产物互斥性被破坏")
	}
}

// INIT-007 多栈走 Moonrepo 并锁定运行时。
func TestINIT007MultiStackRoutesMoon(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("init", "--no-interactive", yes, "--stack", "ts,py,go")
	fixture.AssertExit(t, res, 0)

	if got := fixture.Str(t, res.Envelope, "data.orchestrator"); got != "moon" {
		t.Fatalf("INIT-007 编排器=%s，期望 moon", got)
	}
	for _, p := range []string{"moon.yml", ".tool-versions", "package.json", "tsconfig.json"} {
		if !r.Exists(p) {
			t.Fatalf("INIT-007 缺少 %s", p)
		}
	}
	if r.Exists("nx.json") {
		t.Fatalf("INIT-007 不应同时出现 nx.json")
	}
}

// INIT-008 大团队走 Nx。
func TestINIT008LargeTeamRoutesNx(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run(initArgs("--stack", "ts", "--team-size", "large")...)
	fixture.AssertExit(t, res, 0)

	if got := fixture.Str(t, res.Envelope, "data.orchestrator"); got != "nx" {
		t.Fatalf("INIT-008 编排器=%s，期望 nx", got)
	}
	if !r.Exists("nx.json") || r.Exists("turbo.json") || r.Exists("moon.yml") {
		t.Fatalf("INIT-008 Nx 与其它编排器产物必须互斥")
	}
}

// INIT-009 --dry-run 零写盘且给出完整计划。
func TestINIT009DryRunWritesNothing(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	before := r.Snapshot()

	res := r.Run("init", "--no-interactive", yes, "--stack", "ts", "--dry-run")
	fixture.AssertExit(t, res, 0)
	fixture.AssertNoWrite(t, r, before)

	if len(fixture.Arr(t, res.Envelope, "data.written")) == 0 {
		t.Fatalf("INIT-009 dry-run 应给出计划清单")
	}
	if !fixture.Bool(t, res.Envelope, "data.dry_run") {
		t.Fatalf("INIT-009 dry_run 标记缺失")
	}
}

// INIT-010 --yes --no-interactive 全程零 stdin。
func TestINIT010NonInteractiveNoStdinRead(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run(initArgs("--stack", "ts")...)
	fixture.AssertExit(t, res, 0)
	if !r.Exists("AGENTS.md") {
		t.Fatalf("INIT-010 非交互模式应完成全部生成")
	}
}

// INIT-011 重复执行幂等，不覆盖。
func TestINIT011IdempotentRerun(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run(initArgs("--stack", "ts")...), 0)
	before := r.Snapshot()

	res := r.Run(initArgs("--stack", "ts")...)
	fixture.AssertExit(t, res, 0)
	if len(fixture.Arr(t, res.Envelope, "data.skipped")) == 0 {
		t.Fatalf("INIT-011 二次执行应显式报告跳过项")
	}
	after := r.Snapshot()
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("INIT-011 二次执行覆盖了 %s", p)
		}
	}
}

// INIT-012 非空目录拒绝 init，引导 adopt。
func TestINIT012NonEmptyDirRejected(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Dirty)
	res := r.Run(initArgs("--stack", "ts")...)
	fixture.AssertExit(t, res, 2)

	msg := fixture.Str(t, res.Envelope, "error.message") + " " + fixture.Str(t, res.Envelope, "error.hint")
	if !strings.Contains(strings.ToLower(msg), "adopt") {
		t.Fatalf("INIT-012 应引导使用 adopt: %s", res.Stdout)
	}
}

// INIT-013 非法 stack 退出 2 并列出合法枚举。
func TestINIT013InvalidStackRejected(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run(initArgs("--stack", "cobol")...)
	fixture.AssertExit(t, res, 2)

	msg := fixture.Str(t, res.Envelope, "error.message")
	if !strings.Contains(msg, "ts") {
		t.Fatalf("INIT-013 应列出合法枚举: %s", msg)
	}
	if r.Exists("AGENTS.md") {
		t.Fatalf("INIT-013 参数非法时不得写盘")
	}
}

// INIT-016 无 git 环境仍完成生成。
func TestINIT016WorksWithoutGit(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	if err := os.RemoveAll(filepath.Join(r.Dir, ".git")); err != nil {
		t.Fatalf("INIT-016 清理 .git 失败: %v", err)
	}
	res := r.Run(initArgs("--stack", "ts")...)
	fixture.AssertExit(t, res, 0)
	if !r.Exists("AGENTS.md") {
		t.Fatalf("INIT-016 缺 git 时仍应生成完整骨架")
	}
}

// INIT-017 路径含空格与中文仍成功。
func TestINIT017UnicodeSpacePath(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "项目 空间")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("INIT-017 建目录失败: %v", err)
	}
	res := fixture.Run(t, dir, t.TempDir(), "init", "--no-interactive", yes, "--stack", "ts")
	fixture.AssertExit(t, res, 0)

	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatalf("INIT-017 含中文空格路径未生成产物: %v", err)
	}
}
