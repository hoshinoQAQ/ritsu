// Package lint 是静态检查编排层。它负责按 scope 选文件、应用规则、聚合去重。
package lint

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ritsu/internal/config"
	"ritsu/internal/contract"
	"ritsu/internal/fsx"
	"ritsu/internal/lint/graph"
)

// Hit 是单条规则命中。
type Hit struct {
	Line    int
	Message string
	Fix     string
	Flow    []string
}

// Rule 是一条可机械检测的禁例。
type Rule struct {
	ID       string
	Engine   string
	Severity string
	Exts     []string
	Scope    []string // 为空表示全仓
	Exclude  []string
	Check    func(rel string, lines []string) []Hit
}

// Linter 实现 contract.Linter。
type Linter struct {
	arch       *config.Architecture
	modulePath string
	Only       string
	Strict     bool
}

// New 构造编排器。
func New(arch *config.Architecture, modulePath string) *Linter {
	return &Linter{arch: arch, modulePath: modulePath}
}

// Run 执行全部规则。返回诊断与已扫描文件数。
func (l *Linter) Run(root string, scope []string) ([]contract.Diagnostic, int, error) {
	// 记录调用方是否真的限定了作用域：依赖图只按此收敛
	graphScope := scope
	if len(graphScope) == 0 || (len(graphScope) == 1 && graphScope[0] == ".") {
		graphScope = nil
	}
	if len(scope) == 0 {
		scope = []string{"."}
	}
	files, err := collect(root, scope)
	if err != nil {
		return nil, 0, err
	}
	if len(files) == 0 {
		return nil, 0, contract.NewErr(contract.CodeEmptyScope,
			"作用域内没有任何可扫描文件: %s", strings.Join(scope, ",")).
			WithHint("确认路径存在；空集上的全绿是假绿")
	}

	var diags []contract.Diagnostic
	for _, rule := range Rules() {
		if l.Only != "" && rule.ID != l.Only {
			continue
		}
		matched := 0
		for _, rel := range files {
			if !ruleApplies(rule, rel) {
				continue
			}
			data, rerr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if rerr != nil {
				continue
			}
			matched++
			lines := strings.Split(string(data), "\n")
			for _, h := range rule.Check(rel, lines) {
				diags = append(diags, contract.Diagnostic{
					Engine:    rule.Engine,
					RuleID:    rule.ID,
					Severity:  rule.Severity,
					File:      rel,
					Line:      h.Line,
					Message:   h.Message,
					FlowSteps: h.Flow,
					FixHint:   h.Fix,
				})
			}
		}
		// 指定单条规则却命中 0 文件：规则本身失效，必须报错
		if l.Only == rule.ID && matched == 0 {
			diags = append(diags, contract.Diagnostic{
				Engine:   rule.Engine,
				RuleID:   "rule-selfproof",
				Severity: "error",
				File:     ".ritsu/architecture.yaml",
				Line:     1,
				Message:  "规则 " + rule.ID + " 作用域内命中 0 文件，规则已失效",
				FixHint:  "检查规则 scope/Exts 配置",
			})
		}
	}

	// 依赖图只在已纳管仓库上跑；--rule 与 --scope 同样必须约束它
	if l.arch != nil {
		res, gerr := graph.BuildScoped(root, l.arch, l.modulePath, graphScope)
		if gerr != nil {
			return nil, 0, gerr
		}
		for _, d := range res.Diagnostics {
			if l.Only != "" && d.RuleID != l.Only {
				continue
			}
			diags = append(diags, d)
		}
	}

	// 目录级文件类型白名单：声明了 extensions 的目录只允许指定类型
	if l.arch != nil && len(l.arch.Extensions) > 0 && (l.Only == "" || l.Only == "ext-whitelist") {
		for _, rel := range files {
			for glob, allow := range l.arch.Extensions {
				if !config.MatchGlob(glob, rel) {
					continue
				}
				if !hasAllowedExt(rel, allow) {
					diags = append(diags, contract.Diagnostic{
						Engine:   "ritsu",
						RuleID:   "ext-whitelist",
						Severity: "error",
						File:     rel,
						Line:     1,
						Message:  "该目录只允许 " + strings.Join(allow, "/") + "，不允许此类型文件",
						FixHint:  "移到对应目录，或改用允许的文件类型",
					})
				}
				break
			}
		}
	}

	diags = dedupe(diags)
	if !l.Strict {
		diags = dropWarnings(diags)
	} else {
		// strict：告警一并视为错误，任何遗留都会让闸门变红
		for i := range diags {
			if diags[i].Severity == "warning" {
				diags[i].Severity = "error"
			}
		}
	}
	return diags, len(files), nil
}

// scopeEntry 是一个已确认存在的扫描起点。
type scopeEntry struct{ dir, file string }

func collect(root string, scope []string) ([]string, error) {
	// 先确认每个 scope 都真实存在，空集或悬空路径一律拒绝
	var wanted []scopeEntry
	for _, s := range scope {
		full := filepath.Join(root, filepath.FromSlash(s))
		info, err := os.Stat(full)
		if err != nil {
			return nil, contract.NewErr(contract.CodeScopeOutside, "作用域不存在: %s", s).
				WithHint("--scope 必须是仓库内已存在的路径")
		}
		if info.IsDir() {
			wanted = append(wanted, scopeEntry{dir: full})
		} else {
			wanted = append(wanted, scopeEntry{file: full})
		}
	}

	var out []string
	seen := map[string]bool{}
	add := func(rel string) {
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}

	// 从 root 遍历：rel 始终是磁盘真实大小写，避免大小写变体绕过规则作用域
	err := fsx.WalkFiles(root, nil, func(p string, _ os.FileInfo) error {
		if !inScope(p, wanted) {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		add(filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// inScope 判断文件是否落在某个 scope 下。路径比较忽略大小写，
// 以适配 Windows / macOS 的大小写不敏感文件系统。
func inScope(full string, wanted []scopeEntry) bool {
	for _, w := range wanted {
		if w.file != "" {
			if strings.EqualFold(full, w.file) {
				return true
			}
			continue
		}
		relToDir, err := filepath.Rel(w.dir, full)
		if err != nil {
			continue
		}
		if relToDir == "." || strings.HasPrefix(relToDir, "..") {
			continue
		}
		return true
	}
	return false
}

// hasAllowedExt 判定文件扩展名是否在白名单内。
// 隐藏文件（.gitkeep 等）不参与类型约束，否则空目录占位文件会误报。
func hasAllowedExt(rel string, allow []string) bool {
	base := filepath.Base(rel)
	if strings.HasPrefix(base, ".") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(rel))
	for _, a := range allow {
		if ext == a {
			return true
		}
	}
	return false
}

func ruleApplies(rule Rule, rel string) bool {
	okExt := false
	for _, e := range rule.Exts {
		if strings.HasSuffix(rel, e) {
			okExt = true
			break
		}
	}
	if !okExt {
		return false
	}
	for _, ex := range rule.Exclude {
		if config.MatchGlob(ex, rel) {
			return false
		}
	}
	if len(rule.Scope) == 0 {
		return true
	}
	for _, s := range rule.Scope {
		if config.MatchGlob(s, rel) {
			return true
		}
	}
	return false
}

// ApplySeverityPolicy 统一告警策略：非 strict 丢弃 warning，strict 升级为 error。
// 内置规则与外部引擎必须共用同一套判定，否则告警能绕过闸门。
func ApplySeverityPolicy(in []contract.Diagnostic, strict bool) []contract.Diagnostic {
	if strict {
		for i := range in {
			if in[i].Severity == "warning" {
				in[i].Severity = "error"
			}
		}
		return in
	}
	return dropWarnings(in)
}

// Dedupe 对外暴露去重，供多引擎聚合时跨源复用同一套判定。
func Dedupe(in []contract.Diagnostic) []contract.Diagnostic { return dedupe(in) }

func dedupe(in []contract.Diagnostic) []contract.Diagnostic {	seen := map[string]bool{}
	out := in[:0]
	for _, d := range in {
		k := d.RuleID + "|" + d.File + "|" + strconv.Itoa(d.Line) + "|" + d.Message
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func dropWarnings(in []contract.Diagnostic) []contract.Diagnostic {
	out := in[:0]
	for _, d := range in {
		if d.Severity == "warning" {
			continue
		}
		out = append(out, d)
	}
	return out
}
