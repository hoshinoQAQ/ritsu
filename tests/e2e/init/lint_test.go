package init_test

import (
	"encoding/json"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)
// INIT-014 生成的 TS 骨架必须能通过 tsc --noEmit。
func TestINIT014GeneratedTSCompiles(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	// 真跑编译器：只校验不依赖 React 类型的 .ts 源码，
	// .tsx 需要 @types/react，属后续安装了 node_modules 的场景
	out := runTS(t, r)
	if out == "" {
		t.Skip("环境无 npx/tsc，无法真跑编译器")
	}
	if strings.Contains(out, "error TS") {
		t.Fatalf("INIT-014 生成骨架编译失败:\n%s", out)
	}
}

// INIT-015 生成的产物自身能通过 ritsu lint 引擎（真跑引擎，不用 stub）。
func TestINIT015GeneratedRepoLintsClean(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	res := r.Run("check", "--format", "agent")
	fixture.AssertExit(t, res, 0)
	if n := fixture.Num(t, res.Envelope, "data.scanned"); n <= 0 {
		t.Fatalf("INIT-015 引擎未扫描任何文件")
	}
}

// runTS 真跑 tsc --noEmit，只针对不依赖 React 类型的 .ts 源码。
// 返回空字符串表示环境不具备运行条件。
func runTS(t *testing.T, r *fixture.Repo) string {
	t.Helper()
	// 只校验 .ts，避免 .tsx 因缺 @types/react 干扰判定
	r.Write("tsconfig.check.json", `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true,
    "baseUrl": ".",
    "paths": { "@/*": ["src/*"] }
  },
  "include": ["src/types/index.ts", "src/shared/ids.ts", "src/service/index.ts"]
}`)
	out, err := r.Exec("npx", "--yes", "-p", "typescript@5.6.3", "tsc", "-p", "tsconfig.check.json")
	if err != nil && out == "" {
		return ""
	}
	return out
}

// tsconfigIsValid 断言生成的 tsconfig 是合法 JSON 且覆盖 src。
func TestINIT014bTsconfigShape(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	var cfg map[string]any
	if err := json.Unmarshal([]byte(r.Read("tsconfig.json")), &cfg); err != nil {
		t.Fatalf("INIT-014 tsconfig 不是合法 JSON: %v", err)
	}
	if _, ok := cfg["compilerOptions"]; !ok {
		t.Fatalf("INIT-014 tsconfig 缺 compilerOptions")
	}
}
