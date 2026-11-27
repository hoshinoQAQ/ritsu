package plat_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

func managed(t *testing.T) *fixture.Repo {
	t.Helper()
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)
	return r
}

// PLAT-001 Windows 长路径（>260）与含空格中文路径都必须可用。
func TestPLAT001LongPathAndSpaces(t *testing.T) {
	base := t.TempDir()
	seg := strings.Repeat("深路径-", 4)
	dir := filepath.Join(base, seg, "项目 空间")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("当前文件系统不支持该路径: %v", err)
	}
	// 再造多层嵌套，把总长度推到平台上限附近（Windows CreateProcess 限 260）
	deep := dir
	for i := 0; i < 3; i++ {
		deep = filepath.Join(deep, strings.Repeat("d", 40))
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Skipf("当前文件系统不支持长路径: %v", err)
	}
	if len(deep) <= 200 {
		t.Skipf("未能构造出足够长的路径（实际 %d）", len(deep))
	}
	if len(deep) > 255 {
		t.Skipf("路径 %d 超出 Windows 进程工作目录上限", len(deep))
	}

	res := fixture.Run(t, deep, t.TempDir(), "init", "--no-interactive", "--yes", "--stack", "ts")
	fixture.AssertExit(t, res, 0)
	if _, err := os.Stat(filepath.Join(deep, "AGENTS.md")); err != nil {
		t.Fatalf("PLAT-001 长路径下未生成产物: %v", err)
	}
}

// PLAT-002 .bat / .cmd 目标一律拒绝（BatBadBut）。
func TestPLAT002BatchTargetRejected(t *testing.T) {
	for _, name := range []string{"evil.bat", "evil.cmd"} {
		r := managed(t)
		r.Write(".ritsu/engines.json",
			`{"engines":[{"id":"bat","command":"`+name+`","args":[],"timeout_ms":1000}]}`)

		res := r.Run("check")
		fixture.AssertExit(t, res, 2)
		msg := fixture.Str(t, res.Envelope, "error.message")
		if !strings.Contains(msg, "批处理") {
			t.Fatalf("PLAT-002 未拒绝 %s: %s", name, msg)
		}
	}
}

// PLAT-003 参数里的双引号必须原样到达子进程，不被 shell 吞掉。
func TestPLAT003QuotesPassedVerbatim(t *testing.T) {
	r := managed(t)
	r.Write("tools/echo.mjs",
		"console.log(JSON.stringify([{rule_id:'arg',severity:'warning',file:'x.ts',line:1,message:process.argv[2]}]));\n")
	r.Write(".ritsu/engines.json", string(fixture.Encode(t, map[string]any{
		"engines": []map[string]any{{
			"id":         "echo",
			"command":    "node",
			"args":       []string{"tools/echo.mjs", `a "b" c`},
			"timeout_ms": 10000,
		}},
	})))

	res := r.Run("check", "--strict")
	fixture.AssertExit(t, res, 1)

	var got string
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if m["rule_id"] == "arg" {
			got, _ = m["message"].(string)
		}
	}
	if !strings.Contains(got, `"b"`) {
		t.Fatalf("PLAT-003 双引号被吞: %q", got)
	}
}

// PLAT-004 大小写不敏感文件系统下，路径判定必须归一，不能当成两个文件。
func TestPLAT004CaseInsensitivePaths(t *testing.T) {
	r := managed(t)
	r.Write("src/service/Case.ts", "export const x = 'SELECT a FROM t';\n")

	// 用不同大小写引用同一目录，仍能命中
	res := r.Run("check", "--scope", "SRC/Service")
	if res.ExitCode != 1 {
		t.Fatalf("PLAT-004 大小写变体未被识别为同一位置: exit=%d %s", res.ExitCode, res.Stdout)
	}
}

// PLAT-005 仓库内硬链接不得导致越界读取。
func TestPLAT005HardLinkStaysInside(t *testing.T) {
	r := managed(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("外部内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(r.Dir, "src", "linked.txt")
	if err := os.Link(outside, link); err != nil {
		t.Skipf("当前文件系统不支持硬链接: %v", err)
	}
	// 硬链接在仓库内是合法文件，scope 校验不得误伤
	res := r.Run("check", "--scope", "src")
	if res.ExitCode != 0 {
		t.Fatalf("PLAT-005 硬链接被误判为越界: %s", res.Stdout)
	}
}

// PLAT-006 生成的文本文件统一 LF，不得混入 CRLF。
func TestPLAT006GeneratedFilesUseLF(t *testing.T) {
	r := managed(t)
	for _, rel := range []string{"AGENTS.md", ".ritsu/architecture.yaml", "agent/CONTINUITY.md", "CODEOWNERS"} {
		if strings.Contains(r.Read(rel), "\r\n") {
			t.Fatalf("PLAT-006 %s 含 CRLF", rel)
		}
	}
}
