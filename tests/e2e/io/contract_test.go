package io_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// TestIO001StdoutIsJSON 任意命令 stdout 可被严格 JSON 解析。
func TestIO001StdoutIsJSON(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("version")
	fixture.AssertExit(t, res, 0)
	if !json.Valid([]byte(res.Stdout)) {
		t.Fatalf("stdout 不是合法 JSON: %q", res.Stdout)
	}
}

// TestIO002SchemaVersionEverywhere 所有命令都带 schemaVersion。
func TestIO002SchemaVersionEverywhere(t *testing.T) {
	cmds := [][]string{
		{"version"},
		{"schema", "init"},
		{"schema", "check"},
		{"init", "--dry-run", "--stack", "ts"},
		{"check", "--scope", "src"},
		{"plan", "--goal", "x", "--dry-run"},
		{"bogus-command"},
	}
	for _, args := range cmds {
		r := fixture.NewRepo(t, fixture.Empty)
		res := r.Run(args...)
		if !fixture.Has(res.Envelope, "schemaVersion") {
			t.Fatalf("%v 缺 schemaVersion: %s", args, res.Stdout)
		}
	}
}

// TestIO003ExitCodesThreeStates 退出码只有 0/1/2。
func TestIO003ExitCodesThreeStates(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)

	// 0：成功
	fixture.AssertExit(t, r.Run("version"), 0)

	// 2：参数错误
	fixture.AssertExit(t, r.Run("check", "--bogus"), 2)

	// 1：运行故障（违规存在）
	r.Write("src/a.ts", "const raw = store.sqlite.prepare('x');\n")
	fixture.AssertExit(t, r.Run("check"), 1)
}

// TestIO004LogsOnlyOnStderr 日志只进 stderr。
func TestIO004LogsOnlyOnStderr(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("init", "--dry-run", "--stack", "ts", "--verbose")
	fixture.AssertExit(t, res, 0)
	if !json.Valid([]byte(strings.TrimSpace(res.Stdout))) {
		t.Fatalf("stdout 混入了非 JSON 内容: %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "level=") {
		t.Fatalf("--verbose 应在 stderr 输出结构化日志，实际: %q", res.Stderr)
	}
}

// TestIO005HumanIgnoredWhenNotTTY 非 TTY 下 --human 被忽略。
func TestIO005HumanIgnoredWhenNotTTY(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("version", "--human")
	fixture.AssertExit(t, res, 0)
	if !json.Valid([]byte(strings.TrimSpace(res.Stdout))) {
		t.Fatalf("非 TTY 下 --human 仍应是 JSON: %q", res.Stdout)
	}
}

// TestIO006NoAnsiControl ANSI 转义被彻底抑制。
func TestIO006NoAnsiControl(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	for _, args := range [][]string{{"version"}, {"check"}, {"init", "--dry-run"}} {
		res := r.Run(args...)
		if strings.Contains(res.Stdout, "\x1b[") || strings.Contains(res.Stderr, "\x1b[") {
			t.Fatalf("%v 输出含 ANSI: stdout=%q stderr=%q", args, res.Stdout, res.Stderr)
		}
	}
}

// TestIO007FieldsPrune --fields 裁剪后其余键不存在。
func TestIO007FieldsPrune(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("version", "--fields", "schemaVersion")
	fixture.AssertExit(t, res, 0)
	if !fixture.Has(res.Envelope, "schemaVersion") {
		t.Fatal("schemaVersion 应保留")
	}
	if fixture.Has(res.Envelope, "version") {
		t.Fatalf("--fields 未生效，version 仍存在: %s", res.Stdout)
	}
}

// TestIO008TruncationWithLogPath 超长输出截断并给出指针。
func TestIO008TruncationWithLogPath(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("const raw = store.sqlite.prepare('q');\n")
	}
	r.Write("src/many.ts", b.String())

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	diags := fixture.Arr(t, res.Envelope, "diagnostics")
	if len(diags) != 30 {
		t.Fatalf("diagnostics 应为 30 条（截断上限），实际 %d", len(diags))
	}
	if v, _ := res.Envelope["truncated"].(bool); !v {
		t.Fatal("应标记 truncated=true")
	}
	p := fixture.Str(t, res.Envelope, "full_log_path")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("full_log_path 指向的文件不存在: %s", p)
	}
}

// TestIO009HelpBounded --help ≤ 50 行且含可直接执行的示例。
func TestIO009HelpBounded(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.RunText("--help")
	fixture.AssertExit(t, res, 0)

	lines := strings.Split(strings.TrimRight(res.Stdout, "\n"), "\n")
	if len(lines) > 50 {
		t.Fatalf("--help 超 50 行: %d", len(lines))
	}
	examples := 0
	for _, line := range lines {
		if strings.Contains(line, "ritsu ") {
			examples++
		}
	}
	if examples < 2 {
		t.Fatalf("--help 至少要有 2 个示例，实际 %d", examples)
	}
}

// TestIO010HelpJSONMatchesSchema --help --json 与 schema 输出等价。
func TestIO010HelpJSONMatchesSchema(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	help := r.Run("--help", "--json")
	schema := r.Run("schema", "ritsu")
	fixture.AssertExit(t, help, 0)

	hf := fixture.Arr(t, help.Envelope, "commands")
	sf := fixture.Arr(t, schema.Envelope, "commands")
	if len(hf) != len(sf) {
		t.Fatalf("命令数不一致: help=%d schema=%d", len(hf), len(sf))
	}
	hm := map[string]bool{}
	for _, v := range hf {
		hm[v.(string)] = true
	}
	for _, v := range sf {
		if !hm[v.(string)] {
			t.Fatalf("schema 有而 help 缺: %s", v)
		}
	}
}

// TestIO011NextActionsExecutable next_actions 可直接执行。
func TestIO011NextActionsExecutable(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	actions := fixture.Arr(t, res.Envelope, "next_actions")
	if len(actions) == 0 {
		t.Fatal("失败时必须给 next_actions")
	}
	executed := 0
	for _, a := range actions {
		s, ok := a.(string)
		if !ok {
			continue
		}
		args := strings.Fields(strings.TrimPrefix(strings.TrimSpace(s), "ritsu "))
		next := r.Run(args...)
		if next.ExitCode == 0 || next.ExitCode == 1 {
			executed++
		}
	}
	if executed == 0 {
		t.Fatal("next_actions 中没有一条可执行")
	}
}

// TestIO012UnknownFlag 未知参数退出 2 并给出合法枚举。
func TestIO012UnknownFlag(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("check", "--bogus")
	fixture.AssertExit(t, res, 2)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "bogus") {
		t.Fatalf("错误信息应指出具体参数: %s", res.Stdout)
	}
}

// TestIO013SecretFlagsRejected 密钥类参数必须拒绝。
func TestIO013SecretFlagsRejected(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	for _, flag := range []string{"--password", "--token", "--api-key"} {
		res := r.Run("init", flag, "x")
		fixture.AssertExit(t, res, 2)
		msg := fixture.Str(t, res.Envelope, "error.message")
		if !strings.Contains(msg, "env") && !strings.Contains(msg, "stdin") {
			t.Fatalf("%s 的提示应引导用 env 或 stdin: %s", flag, msg)
		}
	}
}

// TestIO014ScopeOutsideRepo --scope 指向仓库外必须拒绝。
func TestIO014ScopeOutsideRepo(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	res := r.Run("check", "--scope", "../../etc")
	fixture.AssertExit(t, res, 2)
}

// TestIO015DryRunZeroWrite 所有写命令的 --dry-run 零写盘。
func TestIO015DryRunZeroWrite(t *testing.T) {
	for _, args := range [][]string{
		{"init", "--dry-run", "--stack", "ts"},
		{"sync", "--dry-run"},
		{"plan", "--dry-run", "--goal", "x"},
	} {
		r := fixture.NewRepo(t, fixture.Empty)
		before := r.Snapshot()
		r.Run(args...)
		fixture.AssertNoWrite(t, r, before)
	}
}

// TestIO016NoStrayFiles 每次运行不在仓库留下临时文件。
func TestIO016NoStrayFiles(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	r.Run("init", "--stack", "ts", "--yes")
	for _, p := range []string{".ritsu/tmp", ".ritsu/.tmp"} {
		if r.Exists(p) {
			t.Fatalf("残留临时目录: %s", p)
		}
	}
	files, _ := filepath.Glob(filepath.Join(r.Dir, "*.tmp"))
	if len(files) > 0 {
		t.Fatalf("残留临时文件: %v", files)
	}
}
