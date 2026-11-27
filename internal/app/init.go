// Package app 是用例层：唯一允许编排 L0–L4 的地方。
// 入口适配层只能调本包，保证 CLI 与 MCP 行为一致。
package app

import (
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"ritsu/internal/config"
	"ritsu/internal/contract"
	"ritsu/internal/fsx"
	"ritsu/internal/lint"
	"ritsu/internal/lint/graph"
	"ritsu/internal/logx"
	"ritsu/internal/ownership"
	"ritsu/internal/ratchet"
	"ritsu/internal/template"
)

// Deps 是注入的端口集合。
type Deps struct {
	FS     *fsx.FS
	Exec   contract.Executor
	Clock  contract.Clock
	VCS    contract.VCS
	Root   string
	Module string
}

// InitOptions 是 init 参数。
type InitOptions struct {
	Stack    []string
	TeamSize string
	DryRun   bool
	Yes      bool
}

// legalStacks 是 --stack 的合法枚举。
var legalStacks = []string{"ts", "js", "py", "go", "rust", "java"}

func validateStacks(st []string) error {
	for _, s := range st {
		v := strings.ToLower(strings.TrimSpace(s))
		ok := false
		for _, l := range legalStacks {
			if v == l {
				ok = true
				break
			}
		}
		if !ok {
			return contract.NewErr(contract.CodeUsage,
				"非法 --stack 值: %s；合法枚举: %s", s, strings.Join(legalStacks, ", "))
		}
	}
	return nil
}

// Init 从空仓库生成受治理项目骨架。
func Init(d Deps, o InitOptions) (*contract.Envelope, error) {
	env := contract.OK("init")
	logx.Info("init 开始：stack=%v team_size=%s dry_run=%v",
		o.Stack, o.TeamSize, o.DryRun)

	if err := validateStacks(o.Stack); err != nil {
		return nil, err
	}

	managed := d.FS.Exists(d.Root, ".ritsu/answers.yml")
	if !managed {
		if files, err := d.FS.List(d.Root, "."); err == nil && len(files) > 0 {
			return nil, contract.NewErr(contract.CodeUsage,
				"目录非空，不能用 init 初始化").
				WithHint("对既有项目请用 ritsu adopt")
		}
	}

	rendered := template.Render(template.Options{Stack: o.Stack, TeamSize: o.TeamSize})
	sort.Slice(rendered, func(i, j int) bool { return rendered[i].Path < rendered[j].Path })

	var planned, skipped []string
	for _, f := range rendered {
		if d.FS.Exists(d.Root, f.Path) {
			skipped = append(skipped, f.Path)
			continue
		}
		planned = append(planned, f.Path)
		if o.DryRun {
			continue
		}
		if ownership.IsReadOnly(f.Path) {
			f.Content = f.Content + "\n# read-only: managed by ritsu\n"
		}
		if err := d.FS.Write(d.Root, f.Path, []byte(f.Content)); err != nil {
			return nil, err
		}
	}

	if !o.DryRun {
		// architecture.yaml 的 min_edges 取当前真实边数，作为只增不减的下限
		if err := writeArchitecture(d); err != nil {
			return nil, err
		}
		if _, err := ownership.Refresh(d.FS, d.Root, template.Tier1Paths()); err != nil {
			return nil, err
		}
		if err := snapshotBase(d); err != nil {
			return nil, err
		}
		if err := installSelf(d); err != nil {
			return nil, err
		}
	}

	env.Data = map[string]any{
		"written":    planned,
		"skipped":    skipped,
		"orchestrator": template.Orchestrator(template.Options{Stack: o.Stack, TeamSize: o.TeamSize}),
		"dry_run":    o.DryRun,
	}
	env.WithNext("ritsu check --format agent", "ritsu plan --goal \"<你的目标>\"")
	return env, nil
}

// writeArchitecture 先量真实边数，再写声明，避免下限拍脑袋。
func writeArchitecture(d Deps) error {
	probe := &config.Architecture{
		Order: []string{"types", "config", "db", "service", "runtime", "ui", "api"},
		Paths: map[string][]string{
			"types": {"src/types/**"}, "config": {"src/config/**"},
			"db": {"src/db/**"}, "service": {"src/service/**"},
			"runtime": {"src/runtime/**"}, "ui": {"src/ui/**"},
			"api": {"src/api/**"},
		},
		MinEdges: 0,
	}
	res, err := graph.Build(d.Root, probe, d.Module)
	if err != nil {
		return err
	}
	edges := len(res.Edges)
	var b strings.Builder
	b.WriteString("order: [types, config, db, service, runtime, ui, api]\npaths:\n")
	for _, layer := range probe.Order {
		b.WriteString("  " + layer + ": [")
		for i, p := range probe.Paths[layer] {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p)
		}
		b.WriteString("]\n")
	}
	b.WriteString("graph:\n  min_edges: " + strconv.Itoa(edges) + "\n")
	return d.FS.Write(d.Root, ".ritsu/architecture.yaml", []byte(b.String()))
}

// baseTier2 是需要三向合并的基准文件。
var baseTier2 = []string{
	"AGENTS.md", "package.json", "tsconfig.json",
	"turbo.json", "nx.json", "moon.yml", ".ritsu/answers.yml",
}

// snapshotBase 记录 Tier2 基准副本，供 sync 三向合并使用。
func snapshotBase(d Deps) error {
	for _, rel := range baseTier2 {
		if !d.FS.Exists(d.Root, rel) {
			continue
		}
		data, err := d.FS.Read(d.Root, rel)
		if err != nil {
			continue
		}
		if err := d.FS.Write(d.Root, ".ritsu/base/"+rel, data); err != nil {
			return err
		}
	}
	return nil
}

// AdoptOptions 是 adopt 参数。
type AdoptOptions struct {
	Stack  []string
	DryRun bool
	Reason string
}

// Adopt 把既有项目纳入治理：生成骨架、量边数、记存量债。
func Adopt(d Deps, o AdoptOptions) (*contract.Envelope, error) {
	env := contract.OK("adopt")

	if d.FS.Exists(d.Root, ".ritsu/answers.yml") {
		return nil, contract.NewErr(contract.CodeExists,
			"该仓库已被纳管").WithHint("直接用 ritsu sync 推送上游更新")
	}
	clean, err := d.VCS.IsClean(d.Root)
	if err != nil {
		return nil, err
	}
	if !clean {
		return nil, contract.NewErr(contract.CodeNotAClean,
			"工作树有未提交改动，拒绝纳管").
			WithHint("先提交或 stash，再运行 adopt")
	}

	rendered := template.Render(template.Options{Stack: o.Stack})
	var written []string
	for _, f := range rendered {
		if f.Path == ".ritsu/architecture.yaml" {
			continue // 由 writeArchitecture 依真实边数生成
		}
		if d.FS.Exists(d.Root, f.Path) {
			continue // 绝不覆盖既有文件
		}
		written = append(written, f.Path)
		if o.DryRun {
			continue
		}
		if err := d.FS.Write(d.Root, f.Path, []byte(f.Content)); err != nil {
			return nil, err
		}
	}

	var diags []contract.Diagnostic
	if !o.DryRun {
		if err := writeArchitecture(d); err != nil {
			return nil, err
		}
		if _, err := ownership.Refresh(d.FS, d.Root, template.Tier1Paths()); err != nil {
			return nil, err
		}
		if err := snapshotBase(d); err != nil {
			return nil, err
		}
		if err := installSelf(d); err != nil {
			return nil, err
		}
		found, _, lerr := scan(d, []string{"."}, "")
		if lerr != nil {
			return nil, lerr
		}
		diags = found
		baseline := ratchet.Build(diags, o.Reason)
		if err := ratchet.Save(d.FS, d.Root, baseline); err != nil {
			return nil, err
		}
		// 纳管产物必须进入索引，否则下一次 adopt 会因脏工作树被拒
		if err := d.VCS.AddRetry(d.Root, written...); err != nil {
			return nil, err
		}
		env.Data = map[string]any{
			"written":  written,
			"baseline": len(baseline.Entries),
			"dry_run":  o.DryRun,
		}
		env.WithNext("ritsu check --format agent")
		return env, nil
	}
	env.Data = map[string]any{"written": written, "dry_run": true}
	return env, nil
}

// installSelf 把当前运行的 ritsu 复制进项目，使 npm 脚本无需 PATH 即可调用。
// 已是项目内二进制或读取失败时静默跳过：init 不应因分发问提而失败。
func installSelf(d Deps) error {
	exePath, err := os.Executable()
	if err != nil {
		return nil
	}
	name := "ritsu"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	rel := ".ritsu/bin/" + name
	if d.FS.Exists(d.Root, rel) {
		return nil
	}
	data, rerr := os.ReadFile(exePath)
	if rerr != nil {
		return nil
	}
	return d.FS.Write(d.Root, rel, data)
}

// scan 加载分层声明后运行检查。
func scan(d Deps, scope []string, rule string) ([]contract.Diagnostic, int, error) {
	arch, err := config.LoadArchitecture(d.Root)
	if err != nil {
		var ce *contract.Err
		if ok := asErr(err, &ce); ok && ce.Code == contract.CodeNotFound {
			arch = nil
		} else {
			return nil, 0, err
		}
	}
	l := lint.New(arch, d.Module)
	l.Only = rule
	return l.Run(d.Root, scope)
}

func asErr(err error, target **contract.Err) bool {
	e := contract.As(err)
	if e == nil {
		return false
	}
	*target = e
	return true
}
