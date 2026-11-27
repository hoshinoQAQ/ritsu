// Package config 是配置与环境变量的唯一读取入口。
// 缺必填字段一律返回退出码 2（Fail-Closed），绝不静默兜底。
package config

import (
	"fmt"
	"os"
	"strings"

	"ritsu/internal/contract"
)

// Architecture 是 .ritsu/architecture.yaml 的内存形态。
type Architecture struct {
	Order    []string
	Paths    map[string][]string
	MinEdges int
	// Extensions 是目录级文件类型白名单：glob -> 允许的扩展名。
	// 未在 extensions 里声明的目录不受约束。
	Extensions map[string][]string
}

// Env 读取环境变量。全仓唯一允许读 env 的地方。
func Env(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

// LoadArchitecture 解析自举分层声明。文件缺失视为「未纳管」。
func LoadArchitecture(root string) (*Architecture, error) {
	path := root + string(os.PathSeparator) + ".ritsu" + string(os.PathSeparator) + "architecture.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, contract.NewErr(contract.CodeNotFound, "缺少 .ritsu/architecture.yaml").
			WithHint("先运行 ritsu init 或 ritsu adopt")
	}
	m, err := parseYAML(string(data))
	if err != nil {
		return nil, contract.NewErr(contract.CodeUsage, "architecture.yaml 解析失败: %v", err)
	}
	a := &Architecture{Paths: map[string][]string{}}
	order, ok := m["order"].([]string)
	if !ok || len(order) == 0 {
		return nil, contract.NewErr(contract.CodeUsage, "architecture.yaml 缺 order 字段")
	}
	a.Order = order
	paths, ok := m["paths"].(map[string]any)
	if !ok {
		return nil, contract.NewErr(contract.CodeUsage, "architecture.yaml 缺 paths 字段")
	}
	for k, v := range paths {
		switch t := v.(type) {
		case []string:
			a.Paths[k] = t
		case string:
			a.Paths[k] = []string{t}
		default:
			return nil, contract.NewErr(contract.CodeUsage, "paths.%s 类型不支持", k)
		}
	}
	for _, layer := range a.Order {
		if len(a.Paths[layer]) == 0 {
			return nil, contract.NewErr(contract.CodeUsage,
				"architecture.yaml 声明了层 %q 但没有 paths", layer)
		}
	}
	// Fail-Closed：paths 里出现 order 之外的层，说明声明已经不自洽
	for layer := range a.Paths {
		found := false
		for _, o := range a.Order {
			if o == layer {
				found = true
				break
			}
		}
		if !found {
			return nil, contract.NewErr(contract.CodeUsage,
				"architecture.yaml 的 paths.%s 未出现在 order 中", layer)
		}
	}
	// extensions 可选：目录 -> 允许的扩展名。声明了就只允许这些类型。
	if raw, ok := m["extensions"].(map[string]any); ok {
		a.Extensions = map[string][]string{}
		for glob, v := range raw {
			key := strings.Trim(strings.TrimSpace(glob), `"'`)
			var exts []string
			switch t := v.(type) {
			case []string:
				exts = t
			case string:
				exts = []string{t}
			default:
				return nil, contract.NewErr(contract.CodeUsage, "extensions.%s 类型不支持", key)
			}
			if len(exts) == 0 {
				return nil, contract.NewErr(contract.CodeUsage,
					"extensions.%s 不能为空数组", key)
			}
			for i := range exts {
				e := strings.Trim(strings.TrimSpace(exts[i]), `"'`)
				if !strings.HasPrefix(e, ".") {
					e = "." + e
				}
				exts[i] = strings.ToLower(e)
			}
			a.Extensions[key] = exts
		}
	}
	graph, ok := m["graph"].(map[string]any)
	if !ok {
		return nil, contract.NewErr(contract.CodeUsage, "architecture.yaml 缺 graph 字段")
	}
	me, ok := graph["min_edges"].(string)
	if !ok {
		return nil, contract.NewErr(contract.CodeUsage, "architecture.yaml 缺 graph.min_edges")
	}
	var n int
	if _, err := fmt.Sscanf(me, "%d", &n); err != nil || n < 0 {
		return nil, contract.NewErr(contract.CodeUsage, "graph.min_edges 必须是非负整数")
	}
	a.MinEdges = n
	return a, nil
}

// Rank 返回某路径所属层的序号；不属于任何层返回 -1。
func (a *Architecture) Rank(rel string) (int, string) {
	for i, layer := range a.Order {
		for _, glob := range a.Paths[layer] {
			if MatchGlob(glob, rel) {
				return i, layer
			}
		}
	}
	return -1, ""
}

// MatchGlob 支持 ** 与 *，用于目录 glob 规则。
func MatchGlob(glob, rel string) bool {
	glob = strings.ReplaceAll(glob, "\\", "/")
	rel = strings.ReplaceAll(rel, "\\", "/")
	gp := strings.Split(glob, "/")
	rp := strings.Split(rel, "/")
	return matchParts(gp, rp)
}

func matchParts(gp, rp []string) bool {
	for len(gp) > 0 {
		if gp[0] == "**" {
			if len(gp) == 1 {
				return true
			}
			for i := 0; i <= len(rp); i++ {
				if matchParts(gp[1:], rp[i:]) {
					return true
				}
			}
			return false
		}
		if len(rp) == 0 {
			return false
		}
		if gp[0] != "*" && gp[0] != rp[0] {
			return false
		}
		gp, rp = gp[1:], rp[1:]
	}
	return len(rp) == 0
}

// parseYAML 是受限 YAML 子集解析器：只支持 key、缩进嵌套、内联数组。
// 不引入第三方依赖，保持单二进制交付。
func parseYAML(src string) (map[string]any, error) {
	root := map[string]any{}
	stack := []map[string]any{root}
	indents := []int{-1}

	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		body := strings.TrimSpace(line)
		idx := strings.Index(body, ":")
		if idx < 0 {
			return nil, fmt.Errorf("无法解析行: %q", body)
		}
		key := strings.TrimSpace(body[:idx])
		val := strings.TrimSpace(body[idx+1:])

		for len(indents) > 1 && indent <= indents[len(indents)-1] {
			stack = stack[:len(stack)-1]
			indents = indents[:len(indents)-1]
		}
		cur := stack[len(stack)-1]

		if val == "" {
			child := map[string]any{}
			cur[key] = child
			stack = append(stack, child)
			indents = append(indents, indent)
			continue
		}
		cur[key] = parseScalar(val)
	}
	return root, nil
}

func parseScalar(v string) any {
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		inner := strings.TrimSpace(v[1 : len(v)-1])
		if inner == "" {
			return []string{}
		}
		parts := strings.Split(inner, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			out = append(out, strings.Trim(strings.TrimSpace(p), `"'`))
		}
		// 不排序：order 字段的数组顺序本身就是层级语义
		return out
	}
	return strings.Trim(v, `"'`)
}
