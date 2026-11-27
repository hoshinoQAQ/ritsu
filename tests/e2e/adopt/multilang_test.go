package adopt_test

import (
	"testing"

	"ritsu/tests/e2e/fixture"
)

// ADOPT-008 多语言仓库纳管时路由 Moonrepo，三端配置都在。
func TestADOPT008MultiLangRoutesMoon(t *testing.T) {
	r := fixture.NewRepo(t, fixture.LegacyTS)
	r.Write("pyproject.toml", "[project]\nname = \"legacy\"\n")
	r.Write("go.mod", "module legacy\n\ngo 1.22\n")
	r.CommitAll("chore: multi-lang")

	res := r.Run("adopt", "--stack", "ts,py,go", "--yes")
	fixture.AssertExit(t, res, 0)

	if fixture.Num(t, res.Envelope, "data.baseline") <= 0 {
		t.Fatalf("ADOPT-008 未生成存量账本")
	}
	if !r.Exists("moon.yml") || !r.Exists(".tool-versions") {
		t.Fatalf("ADOPT-008 未路由到 Moonrepo")
	}
	if r.Exists("nx.json") {
		t.Fatalf("ADOPT-008 不应出现 nx.json")
	}
	// 既有三端配置必须原样保留
	for _, rel := range []string{"pyproject.toml", "go.mod", "package.json"} {
		if !r.Exists(rel) {
			t.Fatalf("ADOPT-008 既有配置被删除: %s", rel)
		}
	}
	// 纳管后必须可检查
	res = r.Run("check")
	if res.ExitCode != 0 {
		t.Fatalf("ADOPT-008 纳管后 check 未全绿: %s", res.Stdout)
	}
}
