// Package graph 解析模块依赖图并按层校验方向。
// 它必须能自证：扫描到的边数低于下限时必然报错，否则空集也会全绿。
package graph

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ritsu/internal/config"
	"ritsu/internal/contract"
	"ritsu/internal/fsx"
)

// Edge 是一条依赖边。
type Edge struct {
	From string // 源文件（slash 相对路径）
	To   string // 目标文件或包目录
}

// Result 是图的构建结果。
type Result struct {
	Edges       []Edge
	Diagnostics []contract.Diagnostic
}

// Build 构建整仓依赖图并校验 rank 单调性。
func Build(root string, arch *config.Architecture, modulePath string) (*Result, error) {
	return BuildScoped(root, arch, modulePath, nil)
}

// BuildScoped 只扫描 scope 内的文件；scope 为空表示整仓。
// 限定时不校验 min_edges：局部扫描边数天然少于全仓，报自证失效是误报。
func BuildScoped(root string, arch *config.Architecture, modulePath string, scope []string) (*Result, error) {
	res := &Result{}
	full := len(scope) == 0

	err := fsx.WalkFiles(root, []string{".ritsu"}, func(p string, _ os.FileInfo) error {
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !full && !withinScope(rel, scope) {
			return nil
		}
		switch {
		case strings.HasSuffix(rel, ".go"):
			for _, imp := range goImports(p) {
				target, ok := resolveGo(imp, modulePath)
				if !ok {
					continue
				}
				if target == rel {
					continue
				}
				res.Edges = append(res.Edges, Edge{From: rel, To: target})
			}
		case strings.HasSuffix(rel, ".ts"), strings.HasSuffix(rel, ".tsx"):
			for _, spec := range tsImports(p) {
				target, ok := resolveTS(spec, rel, root)
				if !ok {
					continue
				}
				if target == rel {
					continue
				}
				res.Edges = append(res.Edges, Edge{From: rel, To: target})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(res.Edges, func(i, j int) bool {
		if res.Edges[i].From != res.Edges[j].From {
			return res.Edges[i].From < res.Edges[j].From
		}
		return res.Edges[i].To < res.Edges[j].To
	})

	// rank 单调校验：允许 rank(源) >= rank(目标)
	for _, e := range res.Edges {
		srcRank, srcLayer := arch.Rank(e.From)
		dstRank, dstLayer := arch.Rank(e.To)
		if srcRank < 0 || dstRank < 0 {
			continue
		}
		if srcRank == dstRank {
			continue // 同层默认放行，由 same_layer 规则单独收紧
		}
		if srcRank < dstRank {
			res.Diagnostics = append(res.Diagnostics, contract.Diagnostic{
				Engine:   "graph",
				RuleID:   "layer-direction",
				Severity: "error",
				File:     e.From,
				Line:     findImportLine(root, modulePath, e.From, e.To),
				Message:  "反向依赖：" + srcLayer + " 不得依赖更高层的 " + dstLayer,
				FlowSteps: []string{
					"1. [source] " + e.From + "（层 " + srcLayer + "）",
					"2. [sink] " + e.To + "（层 " + dstLayer + "）",
				},
				FixHint: "把被依赖能力下沉到 " + srcLayer + " 或更低的层",
			})
		}
	}

	// 自证：边数低于下限即视为扫描器失效（仅整仓扫描有意义）
	if full && len(res.Edges) < arch.MinEdges {
		res.Diagnostics = append(res.Diagnostics, contract.Diagnostic{
			Engine:   "graph",
			RuleID:   "graph-selfproof",
			Severity: "error",
			File:     ".ritsu/architecture.yaml",
			Line:     1,
			Message:  "解析到的依赖边数 " + strconv.Itoa(len(res.Edges)) + " 低于下限 " + strconv.Itoa(arch.MinEdges),
			FixHint:  "检查 paths 配置与 tsconfig 别名是否写错；不要下调 min_edges 掩盖问题",
		})
	}
	return res, nil
}

// withinScope 判断文件是否落在某个 scope 下。scope 为目录前缀或具体文件。
func withinScope(rel string, scope []string) bool {
	for _, s := range scope {
		s = strings.TrimSuffix(strings.ReplaceAll(s, "\\", "/"), "/")
		if s == "" || s == "." {
			return true
		}
		if rel == s || strings.HasPrefix(rel, s+"/") {
			return true
		}
	}
	return false
}

func goImports(path string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	var out []string
	for _, imp := range f.Imports {
		v, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func resolveGo(imp, modulePath string) (string, bool) {
	if modulePath == "" || !strings.HasPrefix(imp, modulePath) {
		return "", false
	}
	rest := strings.TrimPrefix(imp, modulePath)
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return "cmd/ritsu", true
	}
	return rest, true
}

// tsImports 扫描 import 语句里的模块说明符。只做词法级提取，不做 AST。
func tsImports(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		out = append(out, tsImportsOnLine(line)...)
	}
	return out
}

// tsImportsOnLine 从单行提取模块说明符，供行号定位逐行复用。
func tsImportsOnLine(line string) []string {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "import ") && !strings.HasPrefix(t, "export ") {
		return nil
	}
	var out []string
	for _, quote := range []string{`"`, `'`} {
		if i := strings.Index(t, "from "+quote); i >= 0 {
			rest := t[i+len("from "+quote):]
			if j := strings.Index(rest, quote); j > 0 {
				out = append(out, rest[:j])
			}
		}
	}
	if strings.HasPrefix(t, "import ") && !strings.Contains(t, " from ") {
		for _, quote := range []string{`"`, `'`} {
			if i := strings.Index(t, quote); i >= 0 {
				rest := t[i+1:]
				if j := strings.Index(rest, quote); j > 0 {
					out = append(out, rest[:j])
				}
			}
		}
	}
	return out
}

// resolveTS 把说明符解析为仓库内相对路径。
func resolveTS(spec, fromFile, root string) (string, bool) {
	if strings.HasPrefix(spec, "@/") {
		base := strings.TrimPrefix(spec, "@/")
		return pickExisting(root, "src/"+base)
	}
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		dir := filepath.ToSlash(filepath.Dir(fromFile))
		joined := filepath.ToSlash(filepath.Join(dir, spec))
		return pickExisting(root, joined)
	}
	return "", false
}

func pickExisting(root, rel string) (string, bool) {
	for _, ext := range []string{"", ".ts", ".tsx", "/index.ts", "/index.tsx"} {
		cand := filepath.ToSlash(filepath.Join(rel, "")) + ext
		cand = strings.TrimSuffix(cand, "/")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(cand))); err == nil {
			return cand, true
		}
	}
	return "", false
}

func findImportLine(root, modulePath, from, to string) int {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(from)))
	if err != nil {
		return 1
	}
	isGo := strings.HasSuffix(from, ".go")
	for i, line := range strings.Split(string(data), "\n") {
		var specs []string
		if isGo {
			specs = quotedOnLine(line)
		} else {
			specs = tsImportsOnLine(line)
		}
		for _, spec := range specs {
			var target string
			var ok bool
			if isGo {
				target, ok = resolveGo(spec, modulePath)
			} else {
				target, ok = resolveTS(spec, from, root)
			}
			if ok && target == to {
				return i + 1
			}
		}
	}
	// 找不到具体 import 行（如语法非常规写法）就回到首行，不伪造行号
	return 1
}

// quotedOnLine 提取一行内的双引号字符串，用于 Go import 行定位。
func quotedOnLine(line string) []string {
	var out []string
	rest := line
	for {
		i := strings.Index(rest, `"`)
		if i < 0 {
			return out
		}
		rest = rest[i+1:]
		j := strings.Index(rest, `"`)
		if j < 0 {
			return out
		}
		out = append(out, rest[:j])
		rest = rest[j+1:]
	}
}
