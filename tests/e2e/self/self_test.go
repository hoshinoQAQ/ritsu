package self_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ritsu/internal/config"
	"ritsu/tests/e2e/fixture"
)

// mirror 复制一份最小可检查的源码树，用于改造后自检，绝不改动真实仓库。
func mirror(t *testing.T) string {
	t.Helper()
	root := fixture.Root(t)
	dst := t.TempDir()
	fixture.CopyTree(t, root, dst, "internal", "cmd", "go.mod", ".ritsu/architecture.yaml")
	return dst
}

func runIn(t *testing.T, dir string, args ...string) fixture.Result {
	t.Helper()
	return fixture.Run(t, dir, t.TempDir(), args...)
}

// SELF-001 ritsu 在自己的仓库上跑 check 必须全绿。
func TestSELF001SelfRepoCheckGreen(t *testing.T) {
	res := fixture.Run(t, fixture.Root(t), t.TempDir(), "check")
	fixture.AssertExit(t, res, 0)
	if n := len(fixture.Arr(t, res.Envelope, "diagnostics")); n != 0 {
		t.Fatalf("SELF-001 自举仓库有 %d 条违规: %s", n, res.Stdout)
	}
}

// SELF-002 architecture.yaml 声明与实际目录一致，无悬空声明也无漏报。
func TestSELF002ArchitectureMatchesDisk(t *testing.T) {
	root := fixture.Root(t)
	arch, err := config.LoadArchitecture(root)
	if err != nil {
		t.Fatalf("SELF-002 自举声明不可解析: %v", err)
	}

	// 每个声明层的每个 glob 都必须至少命中一个真实文件
	for _, layer := range arch.Order {
		for _, glob := range arch.Paths[layer] {
			if !globHits(t, root, glob) {
				t.Fatalf("SELF-002 层 %s 声明的 %s 在盘上不存在", layer, glob)
			}
		}
	}

	// internal 下的每个包目录都必须被某一层覆盖
	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("SELF-002 读 internal 失败: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rel := "internal/" + e.Name() + "/probe.go"
		if rank, layer := arch.Rank(rel); rank < 0 {
			t.Fatalf("SELF-002 internal/%s 未被任何层声明覆盖", e.Name())
		} else {
			_ = layer
		}
	}
}

// SELF-003 删掉 order 中某层必须 Fail-Closed 退出 2。
func TestSELF003MissingLayerFailsClosed(t *testing.T) {
	dir := mirror(t)
	doc := readFile(t, filepath.Join(dir, ".ritsu", "architecture.yaml"))
	broken := strings.Replace(doc, "order: [contract, fsx,", "order: [contract,", 1)
	writeFile(t, filepath.Join(dir, ".ritsu", "architecture.yaml"), broken)

	res := runIn(t, dir, "check")
	fixture.AssertExit(t, res, 2)
	if code := fixture.Str(t, res.Envelope, "error.code"); code == "" {
		t.Fatalf("SELF-003 失败必须带 error.code")
	}
}

// SELF-004 故意制造反向 import 必须退出 1。
func TestSELF004ReverseImportDetected(t *testing.T) {
	dir := mirror(t)
	writeFile(t, filepath.Join(dir, "internal", "contract", "leak.go"),
		"package contract\n\nimport (\n\t\"os\"\n\n\t\"ritsu/internal/app\"\n)\n\nvar _ = os.Args\nvar _ = app.Commands\n")

	res := runIn(t, dir, "check")
	fixture.AssertExit(t, res, 1)

	var hit bool
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if m["rule_id"] == "layer-direction" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("SELF-004 未报告反向依赖: %s", res.Stdout)
	}
}

// SELF-005 指定规则却命中 0 文件，规则自证必须红。
func TestSELF005RuleWithZeroFilesFails(t *testing.T) {
	dir := mirror(t)
	res := runIn(t, dir, "check", "--rule", "no-native-html")
	fixture.AssertExit(t, res, 1)

	var hit bool
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if m["rule_id"] == "rule-selfproof" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("SELF-005 未报告规则自证失败: %s", res.Stdout)
	}
}

// SELF-006 能力独占：在 internal/fsx 之外写盘必须被拦下。
func TestSELF006WriteCapabilityIsExclusive(t *testing.T) {
	dir := mirror(t)
	writeFile(t, filepath.Join(dir, "internal", "app", "rogue.go"),
		"package app\n\nimport \"os\"\n\nfunc rogue(p string) error { return os.WriteFile(p, nil, 0o644) }\n")

	res := runIn(t, dir, "check")
	fixture.AssertExit(t, res, 1)

	var hit bool
	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		if m["rule_id"] == "cap-file-write" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("SELF-006 未拦下越权写盘: %s", res.Stdout)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写 %s 失败: %v", path, err)
	}
}

// globHits 判断某个 glob 在盘上是否至少命中一个文件。
func globHits(t *testing.T, root, glob string) bool {
	t.Helper()
	base := strings.TrimSuffix(glob, "/**")
	dir := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(base, "/*")))
	hit := false
	err := filepath.Walk(dir, func(p string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if config.MatchGlob(glob, filepath.ToSlash(rel)) {
			hit = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return false
	}
	return hit
}
