package perf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ritsu/tests/e2e/fixture"
)

// PERF-001 空仓库 init ≤ 2s。
func TestPERF001InitUnderTwoSeconds(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	// 预热：二进制编译与进程启动不计入被测命令
	fixture.AssertExit(t, r.Run("version"), 0)
	start := time.Now()
	res := r.Run("init", "--no-interactive", "--yes", "--stack", "ts")
	elapsed := time.Since(start)
	fixture.AssertExit(t, res, 0)

	if elapsed > 2*time.Second {
		t.Fatalf("PERF-001 init 耗时 %v，超过 2s", elapsed)
	}
}

// PERF-002 500 文件仓库 check ≤ 5s。
func TestPERF002Check500Files(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	for i := 0; i < 500; i++ {
		r.Write(fmt.Sprintf("src/service/mod%03d.ts", i),
			fmt.Sprintf("export const v%d = %d;\n", i, i))
	}

	start := time.Now()
	res := r.Run("check")
	elapsed := time.Since(start)
	fixture.AssertExit(t, res, 0)

	if elapsed > 5*time.Second {
		t.Fatalf("PERF-002 check 500 文件耗时 %v，超过 5s", elapsed)
	}
}

// PERF-003 5 万文件仓库 check ≤ 90s（p95）。
func TestPERF003Check50kFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("大仓库用例在 -short 下跳过")
	}
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	writeMany(t, r, 50000)

	start := time.Now()
	res := r.Run("check")
	elapsed := time.Since(start)
	fixture.AssertExit(t, res, 0)

	if elapsed > 90*time.Second {
		t.Fatalf("PERF-003 check 5 万文件耗时 %v，超过 90s", elapsed)
	}
}

// PERF-004 依赖图构建 200 包 ≤ 15s。
func TestPERF004Graph200Packages(t *testing.T) {
	r := fixture.NewRepo(t, fixture.Empty)
	fixture.AssertExit(t, r.Run("init", "--no-interactive", "--yes", "--stack", "ts"), 0)

	// 200 个包，每个导入上一层，形成一条 200 层的合法依赖链
	for i := 0; i < 200; i++ {
		imp := ""
		if i > 0 {
			imp = fmt.Sprintf("import { v%d } from './pkg%03d';\n", i-1, i-1)
		}
		r.Write(fmt.Sprintf("src/service/pkg%03d/index.ts", i),
			fmt.Sprintf("%sexport const v%d = %d;\n", imp, i, i))
	}

	start := time.Now()
	res := r.Run("check")
	elapsed := time.Since(start)
	fixture.AssertExit(t, res, 0)

	if elapsed > 15*time.Second {
		t.Fatalf("PERF-004 图构建耗时 %v，超过 15s", elapsed)
	}
}

// writeMany 批量写入极小文件，按目录分桶避免单目录过大。
func writeMany(t *testing.T, r *fixture.Repo, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		bucket := i / 500
		rel := fmt.Sprintf("src/service/b%03d/f%05d.ts", bucket, i)
		r.Write(rel, "export const v = 1;\n")
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "src")); err != nil {
		t.Fatalf("批量写入失败: %v", err)
	}
	_ = strings.TrimSpace("")
}
