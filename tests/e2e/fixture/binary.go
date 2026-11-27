// Package fixture 提供 E2E 测试的临时仓库工厂与 CLI 运行器。
// 这里先于任何生产代码存在：E2E 是判定标准，不是事后补的装饰。
package fixture

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// exeSuffix 在 Windows 必须带 .exe 才能被 exec 拉起；其它平台无需后缀。
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

// Binary 编译一次 ritsu 主程序并返回路径。所有 E2E 共用同一二进制。
func Binary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ritsu-bin-*")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "ritsu"+exeSuffix())
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/ritsu")
		cmd.Dir = repoRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			binErr = &buildError{out: string(out), err: err}
		}
	})
	if binErr != nil {
		t.Fatalf("构建 ritsu 失败: %v", binErr)
	}
	return binPath
}

type buildError struct {
	out string
	err error
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

// Root 返回 ritsu 源码仓库的根目录，供自举用例使用。
func Root(t *testing.T) string {
	t.Helper()
	return repoRoot(t)
}

// CopyTree 把仓库内的若干顶层路径复制到 dst，供自举用例改造后自检。
func CopyTree(t *testing.T, src, dst string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		from := filepath.Join(src, filepath.FromSlash(rel))
		to := filepath.Join(dst, filepath.FromSlash(rel))
		err := filepath.Walk(from, func(p string, info os.FileInfo, werr error) error {
			if werr != nil {
				return werr
			}
			sub, rerr := filepath.Rel(from, p)
			if rerr != nil {
				return rerr
			}
			target := filepath.Join(to, sub)
			if info.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			if rerr := os.MkdirAll(filepath.Dir(target), 0o755); rerr != nil {
				return rerr
			}
			return os.WriteFile(target, data, 0o644)
		})
		if err != nil {
			t.Fatalf("复制 %s 失败: %v", rel, err)
		}
	}
}

// repoRoot 向上寻找 go.mod。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到 go.mod")
		}
		dir = parent
	}
}
