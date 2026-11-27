package verify_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ritsu/tests/e2e/fixture"
)

func initRepo(t *testing.T) *fixture.Repo {
	t.Helper()
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)
	r.CommitAll("chore: skeleton")
	return r
}

// writeEvidence 写证据链与两份日志。
func writeEvidence(t *testing.T, r *fixture.Repo, over map[string]any) {
	t.Helper()
	r.Write(".ritsu/logs/red.txt", "--- FAIL: TestX\n")
	r.Write(".ritsu/logs/green.txt", "--- PASS: TestX\n")
	doc := map[string]any{
		"intent":        "实现支付",
		"scope":         "src",
		"plan":          "docs/exec-plans/active/pay.md",
		"changes":       []string{"src/service/pay.ts"},
		"red_command":   "go test ./pay/...",
		"red_log":       ".ritsu/logs/red.txt",
		"red_time":      "2026-01-01T00:00:00Z",
		"green_command": "go test ./pay/...",
		"green_log":     ".ritsu/logs/green.txt",
		"green_time":    "2026-01-01T01:00:00Z",
		"side_effects":  []string{},
		"warnings":      []string{},
	}
	for k, v := range over {
		doc[k] = v
	}
	r.Write(".ritsu/redgreen.json", string(fixture.Encode(t, doc)))
}

// VERIFY-004 红灯时间晚于绿灯（先实现后补测试）必须红。
func TestVERIFY004RedAfterGreenRejected(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, map[string]any{
		"red_time":   "2026-01-01T05:00:00Z",
		"green_time": "2026-01-01T01:00:00Z",
	})

	res := r.Run("verify")
	fixture.AssertExit(t, res, 1)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "红灯时间晚于") {
		t.Fatalf("VERIFY-004 未报时序颠倒: %s", res.Stdout)
	}
}

// VERIFY-005 验收期间用户工作树逐字节不变。
func TestVERIFY005WorktreeIsolation(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, nil)
	before := r.Snapshot()

	res := r.Run("verify")
	fixture.AssertExit(t, res, 0)

	after := r.Snapshot()
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("VERIFY-005 验收改动了工作树: %s", p)
		}
	}
}

// VERIFY-006 评估器不得引用作者上下文之外的内容。
func TestVERIFY006EvaluatorIgnoresAuthorNotes(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, map[string]any{
		"author_notes": "这是作者私下的解释，不应进证据包",
	})
	fixture.AssertExit(t, r.Run("verify"), 0)

	art := r.Read(".ritsu/artifact.json")
	if strings.Contains(art, "作者私下") {
		t.Fatalf("VERIFY-006 artifact 引用了作者笔记")
	}
	for _, k := range []string{"author_notes"} {
		if strings.Contains(art, k) {
			t.Fatalf("VERIFY-006 artifact 含非契约字段 %s", k)
		}
	}
}

// VERIFY-007 真实服务必须能被拉起并通过健康检查。
func TestVERIFY007RealServiceHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	r := initRepo(t)
	writeEvidence(t, r, map[string]any{
		"services": []map[string]any{{"name": "api", "url": srv.URL, "timeout_ms": 3000}},
	})
	fixture.AssertExit(t, r.Run("verify"), 0)

	if !strings.Contains(r.Read(".ritsu/artifact.json"), `"api"`) {
		t.Fatalf("VERIFY-007 artifact 未记录服务探测结果")
	}

	// 服务不可达必须红
	srv.Close()
	writeEvidence(t, r, map[string]any{
		"services": []map[string]any{{"name": "api", "url": srv.URL, "timeout_ms": 500}},
	})
	res := r.Run("verify")
	fixture.AssertExit(t, res, 1)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "api") {
		t.Fatalf("VERIFY-007 未指名不可用服务: %s", res.Stdout)
	}
}

// VERIFY-009 artifact 不得包含任何凭据。
func TestVERIFY009ArtifactRedactsSecrets(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, map[string]any{
		"side_effects": []string{"设置 token=sk-live-9f8e7d6c5b4a 用于联调"},
		"warnings":     []string{"password: hunter2 曾出现在日志"},
	})
	fixture.AssertExit(t, r.Run("verify"), 0)

	art := r.Read(".ritsu/artifact.json")
	for _, bad := range []string{"sk-live-9f8e7d6c5b4a", "hunter2"} {
		if strings.Contains(art, bad) {
			t.Fatalf("VERIFY-009 artifact 泄漏凭据 %q", bad)
		}
	}
	if !strings.Contains(art, "[REDACTED]") {
		t.Fatalf("VERIFY-009 缺少脱敏标记")
	}
}

// VERIFY-010 破坏性 git 命令一律拒绝。
func TestVERIFY010DestructiveGitRejected(t *testing.T) {
	for _, bad := range []string{"git reset --hard HEAD~1", "git push --force origin main", "git clean -fd"} {
		r := initRepo(t)
		writeEvidence(t, r, map[string]any{
			"red_command":   bad,
			"green_command": bad,
		})
		res := r.Run("verify")
		fixture.AssertExit(t, res, 1)
		if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "破坏性") {
			t.Fatalf("VERIFY-010 未拒绝 %q: %s", bad, res.Stdout)
		}
	}
}

// VERIFY-011 验收成功后临时工作区必须被清理。
func TestVERIFY011WorkdirCleanedOnSuccess(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, nil)
	fixture.AssertExit(t, r.Run("verify"), 0)

	if r.Exists(".ritsu/verify-workdir") {
		t.Fatalf("VERIFY-011 成功后临时工作区仍有残留")
	}
}

// VERIFY-012 验收失败时保留现场并给出可读路径。
func TestVERIFY012FailureKeepsScene(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, map[string]any{"red_log": ".ritsu/logs/missing.txt"})

	res := r.Run("verify")
	fixture.AssertExit(t, res, 1)

	scene := filepath.Join(r.Dir, ".ritsu", "verify-workdir", "failure.txt")
	if _, err := os.Stat(scene); err != nil {
		t.Fatalf("VERIFY-012 未保留现场: %v", err)
	}
}

// VERIFY-013 两进程并发 verify 互不干扰。
func TestVERIFY013ConcurrentVerify(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, nil)

	var wg sync.WaitGroup
	results := make([]fixture.Result, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = r.Run("verify")
		}(i)
	}
	wg.Wait()

	for i, res := range results {
		if res.ExitCode != 0 {
			t.Fatalf("VERIFY-013 第 %d 个进程退出码=%d: %s", i, res.ExitCode, res.Stdout)
		}
	}
	if !r.Exists(".ritsu/artifact.json") {
		t.Fatalf("VERIFY-013 未产出证据包")
	}
}

// VERIFY-014 评估器若改动代码必须被检出并阻断。
func TestVERIFY014EvaluatorMutationBlocked(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, nil)

	// 并发窗口内模拟"评估器试图改代码"：改动必须落在快照之后的停顿里
	res := r.RunDuring(700, 1400, func() {
		r.Write("src/service/index.ts", "export const hacked = 1;\n")
	}, "verify")

	fixture.AssertExit(t, res, 1)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "工作树被改动") {
		t.Fatalf("VERIFY-014 未检出越权改动: %s", res.Stdout)
	}
}

// VERIFY-015 --strict 下任何遗留告警都退出 1。
func TestVERIFY015StrictRejectsWarnings(t *testing.T) {
	r := initRepo(t)
	writeEvidence(t, r, map[string]any{"warnings": []string{"性能回归 3%"}})

	fixture.AssertExit(t, r.Run("verify"), 0)
	res := r.Run("verify", "--strict")
	fixture.AssertExit(t, res, 1)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "遗留告警") {
		t.Fatalf("VERIFY-015 未因告警失败: %s", res.Stdout)
	}
}
