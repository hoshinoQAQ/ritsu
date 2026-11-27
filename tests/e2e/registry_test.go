package fixture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// TestRegistryComplete 做文档↔代码双向核对：
// 每条登记 ID 都必须在某个 _test.go 的注释里出现，反之每个出现的 E2E ID 也必须已登记。
func TestRegistryComplete(t *testing.T) {
	root := fixture.Root(t)
	testDir := filepath.Join(root, "tests", "e2e")

	found := map[string]bool{}
	err := filepath.Walk(testDir, func(p string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, id := range scanIDs(string(data)) {
			found[id] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描测试文件失败: %v", err)
	}

	registered := map[string]bool{}
	for _, c := range fixture.Registry {
		if registered[c.ID] {
			t.Fatalf("登记表有重复 ID: %s", c.ID)
		}
		registered[c.ID] = true
		if !found[c.ID] {
			t.Errorf("已登记但没有对应用例: %s", c.ID)
		}
	}
	for id := range found {
		if !registered[id] {
			t.Errorf("用例未登记到 Registry: %s", id)
		}
	}
}

// families 是允许在用例注释里以短写形式出现的族名。
var families = []string{
	"IO", "INIT", "ADOPT", "SYNC", "PLAN", "CHECK", "VERIFY",
	"JOURNEY", "SELF", "FAULT", "PLAT", "PERF",
}

// scanIDs 从源码中提取用例编号，接受 E2E-XXX-nnn 与短写 XXX-nnn 两种写法。
func scanIDs(src string) []string {
	var out []string
	for i := 0; i < len(src); i++ {
		if strings.HasPrefix(src[i:], "E2E-") {
			j := i
			for j < len(src) && isIDChar(src[j]) {
				j++
			}
			if id := src[i:j]; isCaseID(id) {
				out = append(out, id)
			}
			i = j - 1
			continue
		}
		for _, fam := range families {
			if !strings.HasPrefix(src[i:], fam+"-") {
				continue
			}
			j := i
			for j < len(src) && isIDChar(src[j]) {
				j++
			}
			if id := src[i:j]; isCaseID(id) && strings.HasPrefix(id, fam+"-") {
				out = append(out, "E2E-"+id)
			}
			i = j - 1
			break
		}
		// 函数名写法：TestIO001Foo -> E2E-IO-001
		if strings.HasPrefix(src[i:], "Test") {
			if id, n := funcNameID(src[i:]); id != "" {
				out = append(out, id)
				i += n - 1
			}
		}
	}
	return out
}

// funcNameID 把 Test<FAMILY><nnn> 形式的函数名还原为用例编号。
func funcNameID(s string) (string, int) {
	rest := strings.TrimPrefix(s, "Test")
	for _, fam := range families {
		if !strings.HasPrefix(rest, fam) {
			continue
		}
		j := len(fam)
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		digits := rest[len(fam):j]
		if len(digits) < 3 {
			return "", 0
		}
		return "E2E-" + fam + "-" + digits, len(s) - len(rest) + j
	}
	return "", 0
}

// isCaseID 要求形如 <FAMILY>-<编号>，且编号非空。
func isCaseID(id string) bool {
	body := strings.TrimPrefix(id, "E2E-")
	for _, fam := range families {
		if strings.HasPrefix(body, fam+"-") && len(body) > len(fam)+1 {
			return true
		}
	}
	return false
}

func isIDChar(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-'
}
