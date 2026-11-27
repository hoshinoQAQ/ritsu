package self_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// SELF-007 规则与样本必须配对完整：无孤儿样本、无失效规则。
func TestSELF007RuleTestsPaired(t *testing.T) {
	root := fixture.Root(t)
	res := fixture.Run(t, root, t.TempDir(), "check")
	fixture.AssertExit(t, res, 0)

	for _, d := range fixture.Arr(t, res.Envelope, "diagnostics") {
		m, _ := d.(map[string]any)
		switch m["rule_id"] {
		case "rule-test-orphan", "rule-selfproof", "rule-test-malformed", "rule-test-out-of-scope":
			t.Fatalf("SELF-007 规则与样本未配对: %v", m)
		}
	}

	// 每条内置规则都必须有样本文件，缺一即红
	ids := builtinRuleIDs(t)
	if len(ids) < 10 {
		t.Fatalf("SELF-007 内置规则数量异常: %d", len(ids))
	}
	for _, id := range ids {
		rel := filepath.Join(".ritsu", "rule-tests", id+".json")
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("SELF-007 规则 %s 缺少违规样本", id)
		}
	}
}

// builtinRuleIDs 从规则源码里提取全部规则 ID，作为配对核对的事实源。
func builtinRuleIDs(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture.Root(t), "internal", "lint", "rules.go"))
	if err != nil {
		t.Fatalf("SELF-007 读规则源码失败: %v", err)
	}
	var ids []string
	for _, line := range strings.Split(string(data), "\n") {
		idx := strings.Index(line, `ID: "`)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(`ID: "`):]
		end := strings.Index(rest, `"`)
		if end <= 0 {
			continue
		}
		ids = append(ids, rest[:end])
	}
	return ids
}
