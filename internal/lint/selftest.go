package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ritsu/internal/contract"
	"ritsu/internal/fsx"
)

// RuleTestDir 是规则样本目录。每条规则必须有一个必然违规的样本。
const RuleTestDir = ".ritsu/rule-tests"

// RuleTest 是一个规则的违规样本。
type RuleTest struct {
	File   string `json:"file"`
	Snippet string `json:"snippet"`
	Expect int    `json:"expect"`
}

// SelfTest 做规则与样本的双向自证：
// 有样本没规则 = 孤儿样本；有规则但样本不红 = 规则已失效。
func SelfTest(files *fsx.FS, root string) ([]contract.Diagnostic, error) {
	if !files.Exists(root, RuleTestDir) {
		return nil, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(RuleTestDir)))
	if err != nil {
		return nil, contract.NewErr(contract.CodeInternal,
			"读取规则样本目录失败: %v", err)
	}

	known := map[string]Rule{}
	for _, r := range Rules() {
		known[r.ID] = r
	}

	var out []contract.Diagnostic
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		id := strings.TrimSuffix(name, ".json")
		rel := RuleTestDir + "/" + name
		rule, ok := known[id]
		if !ok {
			out = append(out, contract.Diagnostic{
				Engine:   "ritsu",
				RuleID:   "rule-test-orphan",
				Severity: "error",
				File:     rel,
				Line:     1,
				Message:  "样本对应规则 " + id + " 已不存在，规则与样本必须配对",
				FixHint:  "删除该样本，或恢复规则实现",
			})
			continue
		}
		data, rerr := files.Read(root, rel)
		if rerr != nil {
			return nil, rerr
		}
		var tc RuleTest
		if err := json.Unmarshal(data, &tc); err != nil {
			out = append(out, contract.Diagnostic{
				Engine:   "ritsu",
				RuleID:   "rule-test-malformed",
				Severity: "error",
				File:     rel,
				Line:     1,
				Message:  "样本解析失败: " + err.Error(),
				FixHint:  `样本形如 {"file":"a.ts","snippet":"x","expect":1}`,
			})
			continue
		}
		if tc.Expect <= 0 {
			tc.Expect = 1
		}
		if !ruleApplies(rule, tc.File) {
			out = append(out, contract.Diagnostic{
				Engine:   "ritsu",
				RuleID:   "rule-test-out-of-scope",
				Severity: "error",
				File:     rel,
				Line:     1,
				Message:  "样本文件 " + tc.File + " 不在规则 " + id + " 的作用域内",
				FixHint:  "改用规则实际覆盖的扩展名与目录",
			})
			continue
		}
		hits := rule.Check(tc.File, strings.Split(tc.Snippet, "\n"))
		if len(hits) < tc.Expect {
			out = append(out, contract.Diagnostic{
				Engine:   "ritsu",
				RuleID:   "rule-selfproof",
				Severity: "error",
				File:     rel,
				Line:     1,
				Message:  "规则 " + id + " 的反例样本未命中（期望 " + strconv.Itoa(tc.Expect) + "，实际 " + strconv.Itoa(len(hits)) + "）",
				FixHint:  "规则实现已失效或被改坏，修复后才允许放行",
			})
		}
	}
	return out, nil
}
