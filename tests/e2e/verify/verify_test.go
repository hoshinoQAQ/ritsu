package verify_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

const redGreenPath = ".ritsu/redgreen.json"

// evidence 写一份红绿证据链与两份日志，返回仓库。
func evidence(t *testing.T, red, green, redCmd, greenCmd, redTime, greenTime string) *fixture.Repo {
	t.Helper()
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	if red != "" {
		r.Write(".ritsu/logs/red.txt", red)
	}
	if green != "" {
		r.Write(".ritsu/logs/green.txt", green)
	}
	doc := map[string]any{
		"intent":        "实现登录流程",
		"scope":         "src",
		"plan":          "docs/exec-plans/active/login.md",
		"red_command":   redCmd,
		"red_log":       ".ritsu/logs/red.txt",
		"red_time":      redTime,
		"green_command": greenCmd,
		"green_log":     ".ritsu/logs/green.txt",
		"green_time":    greenTime,
		"side_effects":  []string{"无破坏性操作"},
		"changes":       []string{"src/service/login.ts"},
		"warnings":      []string{},
	}
	r.Write(redGreenPath, string(fixture.Encode(t, doc)))
	return r
}

// VERIFY-001 完整证据链通过，artifact 含两份日志路径。
func TestVERIFY001CompleteEvidencePasses(t *testing.T) {
	r := evidence(t,
		"--- FAIL: TestLogin\n    login_test.go:12: 期望 401，实际 200\n",
		"--- PASS: TestLogin\nok\n",
		"go test ./...", "go test ./...",
		"2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z")

	res := r.Run("verify")
	fixture.AssertExit(t, res, 0)

	artifact := r.Read(".ritsu/artifact.json")
	for _, want := range []string{".ritsu/logs/red.txt", ".ritsu/logs/green.txt"} {
		if !strings.Contains(artifact, want) {
			t.Fatalf("VERIFY-001 artifact 缺 %s", want)
		}
	}
}

// VERIFY-002 只有绿灯也必须红。
func TestVERIFY002MissingRedLogFails(t *testing.T) {
	r := evidence(t,
		"",
		"--- PASS: TestLogin\nok\n",
		"go test ./...", "go test ./...",
		"2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z")
	// 红灯日志文件存在但为空，等价于缺失
	r.Write(".ritsu/logs/red.txt", "")

	res := r.Run("verify")
	fixture.AssertExit(t, res, 1)
	if code := fixture.Str(t, res.Envelope, "error.code"); code != "VERIFY_REDGREEN" {
		t.Fatalf("VERIFY-002 error.code=%s", code)
	}
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "red") {
		t.Fatalf("VERIFY-002 未指名缺什么: %s", res.Stdout)
	}
}

// VERIFY-003 红灯来自别的命令即视为作弊。
func TestVERIFY003CommandMismatchFails(t *testing.T) {
	r := evidence(t,
		"--- FAIL: TestLogin\n",
		"--- PASS: TestLogin\n",
		"go test ./ui/...", "go test ./...",
		"2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z")

	res := r.Run("verify")
	fixture.AssertExit(t, res, 1)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "不一致") {
		t.Fatalf("VERIFY-003 未报命令不等价: %s", res.Stdout)
	}
}

// VERIFY-008 artifact 六段齐全。
func TestVERIFY008ArtifactHasSixSections(t *testing.T) {
	r := evidence(t,
		"--- FAIL: TestLogin\n",
		"--- PASS: TestLogin\n",
		"go test ./...", "go test ./...",
		"2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z")
	fixture.AssertExit(t, r.Run("verify"), 0)

	art := fixture.ParseJSON(t, r.Read(".ritsu/artifact.json"))
	for _, k := range []string{"intent", "scope", "plan", "changes", "checks", "side_effects", "warnings"} {
		if _, ok := art[k]; !ok {
			t.Fatalf("VERIFY-008 artifact 缺字段 %s", k)
		}
	}
}
