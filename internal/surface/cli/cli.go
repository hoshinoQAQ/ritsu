// Package cli 是入口适配层：只做参数解析、分发与输出。
// 不含任何业务分支：所有能力都在 app 层。
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ritsu/internal/app"
	"ritsu/internal/contract"
	"ritsu/internal/logx"
)

// Version 是产品版本。
const Version = "0.1.0"

// secretFlags 一律拒绝：只能走环境变量或 stdin。
var secretFlags = []string{"--password", "--token", "--api-key", "--secret"}

type flags struct {
	stack      []string
	teamSize   string
	scope      []string
	fields     []string
	rule       string
	format     string
	goal       string
	field      string
	value      string
	complete   string
	yes        bool
	noInteract bool
	dryRun     bool
	verbose    bool
	strict     bool
	human      bool
	jsonOut    bool
	acceptDebt bool
	skipAnswer bool
	resetCheck bool
	help       bool
}

// Run 解析参数并执行。返回进程退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	logx.SetWriter(stderr)
	d := deps()
	// 上次运行被强杀可能留下原子写的临时文件，任何命令启动时先清干净
	if d.FS != nil {
		d.FS.CleanupTemps(d.Root)
	}

	f, cmd, err := parse(args)
	if err != nil {
		return emit(stdout, contract.Fail(cmdOr(cmd), contract.As(err)))
	}
	logx.SetVerbose(f.verbose)

	if f.help {
		if f.jsonOut {
			env := contract.OK("help")
			env.Commands = app.Commands()
			return emit(stdout, env)
		}
		fmt.Fprint(stdout, helpText())
		return contract.ExitOK
	}

	env, callErr := dispatch(d, cmd, f)
	if callErr != nil {
		env = contract.Fail(cmd, contract.As(callErr))
	}
	// 失败必须给可执行建议：hint 同时提升为 next_actions
	if env.Error != nil && env.Error.Hint != "" && len(env.NextActions) == 0 {
		env.WithNext(env.Error.Hint)
	}
	if len(f.fields) > 0 {
		env.Prune(f.fields)
	}
	code := contract.ExitOK
	if env.Status == "failed" {
		if env.Error != nil {
			code = contract.ExitCodeFor(env.Error.Code)
			logx.Error("%s", env.Error.Message)
		} else {
			code = contract.ExitFailure
		}
	}
	writeOut(stdout, env)
	return code
}

func dispatch(d app.Deps, cmd string, f flags) (*contract.Envelope, error) {
	switch cmd {
	case "version":
		env := contract.OK("version")
		env.Version = Version
		return env, nil
	case "schema":
		env := contract.OK("schema")
		env.Commands = app.Commands()
		env.Fields = app.Fields()
		return env, nil
	case "init":
		return app.Init(d, app.InitOptions{
			Stack: f.stack, TeamSize: f.teamSize, DryRun: f.dryRun, Yes: f.yes,
		})
	case "adopt":
		return app.Adopt(d, app.AdoptOptions{Stack: f.stack, DryRun: f.dryRun})
	case "sync":
		return app.Sync(d, app.SyncOptions{DryRun: f.dryRun, SkipAnswered: f.skipAnswer})
	case "check":
		return app.Check(d, app.CheckOptions{
			Scope: f.scope, Rule: f.rule, Format: f.format,
			Strict: f.strict, AcceptDebt: f.acceptDebt,
		})
	case "verify":
		return app.Verify(d, app.VerifyOptions{Strict: f.strict})
	case "plan":
		if f.complete != "" {
			return app.PlanComplete(d, f.complete)
		}
		if f.resetCheck {
			return app.PlanReset(d)
		}
		return app.PlanCreate(d, f.goal, f.dryRun)
	case "handoff":
		return app.Handoff(d, f.field, f.value)
	default:
		return nil, contract.NewErr(contract.CodeUsage,
			"未知命令: %s", cmd).WithHint("合法命令: " + strings.Join(app.Commands(), ", "))
	}
}

func cmdOr(s string) string {
	if s == "" {
		return "ritsu"
	}
	return s
}

func emit(w io.Writer, env *contract.Envelope) int {
	writeOut(w, env)
	if env.Status == "failed" && env.Error != nil {
		return contract.ExitCodeFor(env.Error.Code)
	}
	return contract.ExitOK
}

func writeOut(w io.Writer, env *contract.Envelope) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

func helpText() string {
	return strings.Join([]string{
		"ritsu — 面向 AI 编程代理的治理脚手架",
		"",
		"用法:",
		"  ritsu <command> [flags]",
		"",
		"命令:",
		"  init      初始化新仓库",
		"  adopt     纳管既有仓库",
		"  sync      增量同步上游模板",
		"  check     静态检查",
		"  verify    红绿证据链校验",
		"  plan      执行计划与接力卡",
		"  handoff   更新接力卡字段",
		"  schema    参数契约自省",
		"  version   版本",
		"",
		"示例:",
		"  ritsu init --stack ts --yes",
		"  ritsu check --format agent",
		"  ritsu plan --goal \"实现 X\"",
		"",
		"常用参数:",
		"  --scope --dry-run --yes --no-interactive",
		"  --format --strict --fields --rule --verbose",
		"  --help --json",
		"",
		"输出契约: stdout 只出 JSON（含 schemaVersion），stderr 只出日志。",
		"退出码: 0 成功 / 1 运行故障 / 2 参数或契约错误。",
	}, "\n") + "\n"
}
