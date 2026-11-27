package plan_test

import (
	"path/filepath"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

func initRepo(t *testing.T) *fixture.Repo {
	t.Helper()
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)
	return r
}

// PLAN-001 生成执行计划并含验收标准段。
func TestPLAN001CreatesPlanWithAcceptance(t *testing.T) {
	r := initRepo(t)
	res := r.Run("plan", "--goal", "实现登录流程")
	fixture.AssertExit(t, res, 0)

	created := fixture.Str(t, res.Envelope, "data.created")
	if !strings.HasPrefix(created, "docs/exec-plans/active/") {
		t.Fatalf("PLAN-001 计划落点错误: %s", created)
	}
	if !strings.Contains(r.Read(created), "## 验收标准") {
		t.Fatalf("PLAN-001 计划缺验收标准段")
	}
}

// PLAN-002 CONTINUITY 恰好八个固定字段。
func TestPLAN002ContinuityFieldsFixed(t *testing.T) {
	r := initRepo(t)
	fixture.AssertExit(t, r.Run("plan", "--goal", "字段校验"), 0)

	want := []string{"当前目标", "验收标准", "首选必读文档", "已作决策", "当前进度", "阻塞项", "下一步行动", "仍需完成的验证"}
	var got []string
	for _, ln := range strings.Split(r.Read("agent/CONTINUITY.md"), "\n") {
		if strings.HasPrefix(ln, "## ") {
			got = append(got, strings.TrimSpace(strings.TrimPrefix(ln, "## ")))
		}
	}
	if len(got) != len(want) {
		t.Fatalf("PLAN-002 字段数 %d，期望 %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PLAN-002 字段顺序/名称不符: %v", got)
		}
	}
}

// PLAN-003 handoff 只改目标字段。
func TestPLAN003HandoffTouchesOneField(t *testing.T) {
	r := initRepo(t)
	fixture.AssertExit(t, r.Run("plan", "--goal", "接力卡"), 0)
	before := sectionMap(r.Read("agent/CONTINUITY.md"))

	res := r.Run("handoff", "--field", "当前进度", "--value", "已完成 3/5")
	fixture.AssertExit(t, res, 0)

	after := sectionMap(r.Read("agent/CONTINUITY.md"))
	for k, v := range before {
		if k == "当前进度" {
			continue
		}
		if after[k] != v {
			t.Fatalf("PLAN-003 handoff 越界修改了字段 %s", k)
		}
	}
	if !strings.Contains(after["当前进度"], "已完成 3/5") {
		t.Fatalf("PLAN-003 目标字段未更新: %q", after["当前进度"])
	}
}

// PLAN-004 --reset-check 返回是否重置与原因。
func TestPLAN004ResetCheckReportsReason(t *testing.T) {
	r := initRepo(t)
	res := r.Run("plan", "--reset-check")
	fixture.AssertExit(t, res, 0)

	if !fixture.Has(res.Envelope, "data.reset") {
		t.Fatalf("PLAN-004 缺 reset 判定")
	}
	if strings.TrimSpace(fixture.Str(t, res.Envelope, "data.reason")) == "" {
		t.Fatalf("PLAN-004 缺原因说明")
	}
}

// PLAN-005 完成计划移入 completed。
func TestPLAN005CompleteMovesPlan(t *testing.T) {
	r := initRepo(t)
	res := r.Run("plan", "--goal", "完成迁移")
	fixture.AssertExit(t, res, 0)
	created := fixture.Str(t, res.Envelope, "data.created")
	name := filepath.Base(created)

	res = r.Run("plan", "--complete", name)
	fixture.AssertExit(t, res, 0)
	if !r.Exists("docs/exec-plans/completed/" + name) {
		t.Fatalf("PLAN-005 未落入 completed")
	}
	if r.Exists(created) {
		t.Fatalf("PLAN-005 active 未移除")
	}
}

// PLAN-006 completed 同名计划拒绝覆盖。
func TestPLAN006CompleteConflictRejected(t *testing.T) {
	r := initRepo(t)
	res := r.Run("plan", "--goal", "同名冲突")
	fixture.AssertExit(t, res, 0)
	created := fixture.Str(t, res.Envelope, "data.created")
	name := filepath.Base(created)

	fixture.AssertExit(t, r.Run("plan", "--complete", name), 0)
	r.Write(created, "# 重新开的同名计划\n")

	fixture.AssertExit(t, r.Run("plan", "--complete", name), 2)
}

func sectionMap(doc string) map[string]string {
	out := map[string]string{}
	cur := ""
	for _, ln := range strings.Split(doc, "\n") {
		if strings.HasPrefix(ln, "## ") {
			cur = strings.TrimSpace(strings.TrimPrefix(ln, "## "))
			out[cur] = ""
			continue
		}
		if cur != "" {
			out[cur] += ln + "\n"
		}
	}
	return out
}
