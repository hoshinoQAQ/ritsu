package fixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Result 是一次 ritsu 调用的可观察结果。
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Envelope map[string]any
	dir      string
}

func hashBytes(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Run 执行 ritsu 并强制校验四条全局不变量（设计文档第五节）。
// 任何一条不满足都直接判定用例失败，不依赖用例作者记得写。
func Run(t *testing.T, dir, home string, args ...string) Result {
	t.Helper()
	res := execute(t, dir, home, args)
	checkInvariants(t, &res, args)
	return res
}

func runText(t *testing.T, dir, home string, args ...string) Result {
	t.Helper()
	res := execute(t, dir, home, args)
	if strings.Contains(res.Stdout, "\x1b[") {
		t.Fatalf("人类输出含 ANSI 转义: %q", res.Stdout)
	}
	return res
}

func execute(t *testing.T, dir, home string, args []string) Result {
	return executeEnv(t, dir, home, nil, args)
}

func executeEnv(t *testing.T, dir, home string, extra []string, args []string) Result {
	t.Helper()
	cmd := exec.Command(Binary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"RITSU_HOME="+home,
		"NO_COLOR=1",
		"TERM=dumb",
		"RITSU_FAKE_NOW=2026-01-01T00:00:00Z",
	)
	cmd.Env = append(cmd.Env, extra...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("执行 ritsu 失败: %v", err)
		}
	}
	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code, dir: dir}
	return res
}

func checkInvariants(t *testing.T, res *Result, args []string) {
	t.Helper()
	full := "ritsu " + strings.Join(args, " ")

	// 不变量 3：退出码只允许 0/1/2
	if res.ExitCode != 0 && res.ExitCode != 1 && res.ExitCode != 2 {
		t.Fatalf("%s: 退出码 %d 超出三态约定\nstderr: %s", full, res.ExitCode, res.Stderr)
	}

	// 不变量 2：stdout 不得含 ANSI 转义
	if strings.Contains(res.Stdout, "\x1b[") {
		t.Fatalf("%s: stdout 含 ANSI 转义", full)
	}

	// 不变量 1：stdout 必须是合法 JSON 且含 schemaVersion
	trimmed := strings.TrimSpace(res.Stdout)
	if trimmed == "" {
		t.Fatalf("%s: stdout 为空，必须返回 JSON 信封\nstderr: %s", full, res.Stderr)
	}
	if !json.Valid([]byte(trimmed)) {
		t.Fatalf("%s: stdout 不是合法 JSON:\n%s", full, trimmed)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		t.Fatalf("%s: 解析信封失败: %v", full, err)
	}
	if _, ok := env["schemaVersion"]; !ok {
		t.Fatalf("%s: 信封缺 schemaVersion: %s", full, trimmed)
	}
	res.Envelope = env

	// 不变量 4：失败时必须有 error.code 与 error.message
	if res.ExitCode != 0 {
		e, ok := env["error"].(map[string]any)
		if !ok {
			t.Fatalf("%s: 退出码 %d 但信封无 error 对象: %s", full, res.ExitCode, trimmed)
		}
		if _, ok := e["code"].(string); !ok {
			t.Fatalf("%s: error 缺 code", full)
		}
		if _, ok := e["message"].(string); !ok {
			t.Fatalf("%s: error 缺 message", full)
		}
	}
}

// ── 断言助手 ──────────────────────────────────────────────

// AssertExit 断言退出码。
func AssertExit(t *testing.T, res Result, want int) {
	t.Helper()
	if res.ExitCode != want {
		t.Fatalf("退出码 = %d, 期望 %d\nstdout: %s\nstderr: %s",
			res.ExitCode, want, res.Stdout, res.Stderr)
	}
}

// AssertNoWrite 断言目录内容零变化。
func AssertNoWrite(t *testing.T, r *Repo, before map[string]string) {
	t.Helper()
	after := r.Snapshot()
	for k, v := range after {
		if before[k] != v {
			t.Fatalf("--dry-run 却改动了文件: %s", k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			t.Fatalf("--dry-run 却删除了文件: %s", k)
		}
	}
}

// AssertChanged 断言指定文件发生了变化。
func AssertChanged(t *testing.T, r *Repo, before map[string]string, rel string) {
	t.Helper()
	after := r.Snapshot()
	if before[rel] == after[rel] {
		t.Fatalf("期望 %s 变化，但内容未变", rel)
	}
}

// AssertUnchanged 断言指定文件未变化。
func AssertUnchanged(t *testing.T, r *Repo, before map[string]string, rel string) {
	t.Helper()
	after := r.Snapshot()
	if before[rel] != after[rel] {
		t.Fatalf("期望 %s 不变，但被改动", rel)
	}
}

// Str 从信封取字符串字段，支持 a.b 路径。
func Str(t *testing.T, env map[string]any, path string) string {
	t.Helper()
	v := get(env, path)
	s, ok := v.(string)
	if !ok {
		t.Fatalf("字段 %s 不是字符串: %#v", path, v)
	}
	return s
}

// Num 取数值字段。
func Num(t *testing.T, env map[string]any, path string) float64 {
	t.Helper()
	v := get(env, path)
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("字段 %s 不是数值: %#v", path, v)
	}
	return f
}

// Arr 取数组字段。
func Arr(t *testing.T, env map[string]any, path string) []any {
	t.Helper()
	v := get(env, path)
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("字段 %s 不是数组: %#v", path, v)
	}
	return a
}

// Has 判断字段路径是否存在。
func Has(env map[string]any, path string) bool {
	return get(env, path) != nil
}

// ParseJSON 解析 JSON 字符串，失败即红。
func ParseJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("JSON 解析失败: %v\n%s", err, body)
	}
	return out
}

// Encode 把对象序列化为 JSON 字符串。
func Encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	return append(b, '\n')
}

// Bool 读取布尔字段。
func Bool(t *testing.T, env map[string]any, path string) bool {
	t.Helper()
	v, ok := get(env, path).(bool)
	if !ok {
		t.Fatalf("字段 %s 不是布尔: %v", path, get(env, path))
	}
	return v
}

func get(env map[string]any, path string) any {
	cur := any(env)
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[seg]
		if !ok {
			return nil
		}
	}
	return cur
}
