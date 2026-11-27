package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ritsu/internal/app"
	"ritsu/internal/clockx"
	"ritsu/internal/config"
	"ritsu/internal/contract"
	"ritsu/internal/execx"
	"ritsu/internal/fsx"
	"ritsu/internal/vcs"
)

// deps 装配全部端口实现。这是唯一知道具体实现的地方。
func deps() app.Deps {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	exec := execx.New(0)
	return app.Deps{
		FS:     fsx.New(debugSlowMS()),
		Exec:   exec,
		Clock:  clockx.New(config.Env("RITSU_FAKE_NOW", "")),
		VCS:    vcs.New(exec),
		Root:   root,
		Module: detectModule(root),
	}
}

// debugSlowMS 读取故障注入延迟，仅供 E2E 构造可中断窗口，生产环境为 0。
func debugSlowMS() int {
	v := config.Env("RITSU_DEBUG_SLOW_MS", "")
	if v == "" {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return 0
	}
	return n
}

// detectModule 从 go.mod 读出模块前缀，用于把 import 路径映射到目录。
func detectModule(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "module "))
		}
	}
	return ""
}

var valueFlags = map[string]bool{
	"--stack": true, "--team-size": true, "--scope": true, "--fields": true,
	"--rule": true, "--format": true, "--goal": true, "--field": true,
	"--value": true, "--complete": true,
}

var boolFlags = map[string]bool{
	"--yes": true, "--no-interactive": true, "--dry-run": true, "--verbose": true,
	"--strict": true, "--human": true, "--json": true, "--accept-debt": true,
	"--skip-answered": true, "--reset-check": true, "--help": true,
}

func parse(args []string) (flags, string, error) {
	f := flags{format: "agent"}
	cmd := ""

	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			if cmd == "" {
				cmd = a
				continue
			}
			if cmd == "schema" {
				continue // schema 可带一个子命令名，仅用于自省
			}
			return f, cmd, contract.NewErr(contract.CodeUsage, "多余的参数: %s", a)
		}

		name := a
		inline := ""
		if idx := strings.Index(a, "="); idx > 0 {
			name = a[:idx]
			inline = a[idx+1:]
		}

		for _, s := range secretFlags {
			if name == s {
				return f, cmd, contract.NewErr(contract.CodeSecretFlag,
					"禁止通过命令行传密钥: %s；请改用环境变量或 stdin 读取", name)
			}
		}

		if boolFlags[name] {
			switch name {
			case "--yes":
				f.yes = true
			case "--no-interactive":
				f.noInteract = true
			case "--dry-run":
				f.dryRun = true
			case "--verbose":
				f.verbose = true
			case "--strict":
				f.strict = true
			case "--human":
				f.human = true
			case "--json":
				f.jsonOut = true
			case "--accept-debt":
				f.acceptDebt = true
			case "--skip-answered":
				f.skipAnswer = true
			case "--reset-check":
				f.resetCheck = true
			case "--help":
				f.help = true
			}
			continue
		}

		if !valueFlags[name] {
			return f, cmd, contract.NewErr(contract.CodeUnknownFlag,
				"未知参数: %s", name).
				WithHint("用 --help --json 查看全部合法参数")
		}

		val := inline
		if val == "" {
			if i+1 >= len(args) {
				return f, cmd, contract.NewErr(contract.CodeUsage, "%s 缺少取值", name)
			}
			i++
			val = args[i]
		}

		switch name {
		case "--stack":
			f.stack = splitList(val)
		case "--team-size":
			f.teamSize = val
		case "--scope":
			f.scope = append(f.scope, splitList(val)...)
		case "--fields":
			f.fields = splitList(val)
		case "--rule":
			f.rule = val
		case "--format":
			f.format = val
		case "--goal":
			f.goal = val
		case "--field":
			f.field = val
		case "--value":
			f.value = val
		case "--complete":
			f.complete = val
		}
	}

	if cmd == "" && !f.help {
		return f, cmd, contract.NewErr(contract.CodeUsage, "缺少命令").
			WithHint("合法命令: " + strings.Join(app.Commands(), ", "))
	}

	if len(f.scope) > 0 {
		if err := validateScope(f.scope); err != nil {
			return f, cmd, err
		}
	}
	return f, cmd, nil
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// validateScope 在入口就把越界路径拒掉，不让它流到内核。
func validateScope(scope []string) error {
	root, err := os.Getwd()
	if err != nil {
		return nil
	}
	f := fsx.New()
	for _, s := range scope {
		if _, err := f.Within(root, s); err != nil {
			return contract.As(err)
		}
	}
	return nil
}
