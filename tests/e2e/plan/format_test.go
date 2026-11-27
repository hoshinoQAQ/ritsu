package plan_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// PLAN-007 --format json 与 --human 语义等价（非 TTY 下 human 被忽略，契约一致）。
func TestPLAN007JSONAndHumanEquivalent(t *testing.T) {
	r := initRepo(t)

	jsonRes := r.Run("plan", "--goal", "语义等价校验")
	fixture.AssertExit(t, jsonRes, 0)
	created := fixture.Str(t, jsonRes.Envelope, "data.created")

	dupJSON := r.Run("plan", "--goal", "语义等价校验")
	fixture.AssertExit(t, dupJSON, 2)

	dupHuman := r.Run("plan", "--goal", "语义等价校验", "--human")
	if dupHuman.ExitCode != dupJSON.ExitCode {
		t.Fatalf("PLAN-007 退出码不一致: json=%d human=%d",
			dupJSON.ExitCode, dupHuman.ExitCode)
	}
	if a, b := fixture.Str(t, dupJSON.Envelope, "error.code"), fixture.Str(t, dupHuman.Envelope, "error.code"); a != b {
		t.Fatalf("PLAN-007 错误码不一致: json=%s human=%s", a, b)
	}
	if !strings.Contains(fixture.Str(t, dupHuman.Envelope, "error.message"), created) {
		t.Fatalf("PLAN-007 human 未提到同一计划路径 %s", created)
	}
}
