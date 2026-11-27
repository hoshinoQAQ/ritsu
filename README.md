# Ritsu

面向 AI 编程代理的治理脚手架。给仓库装上可机械执行的边界：分层依赖、架构守卫、证据链、违规棘轮。

人类靠记忆维持约束，代理靠提示词维持约束。两者都会漂移。Ritsu 把约束变成 `ritsu check` 能判定的东西——不通过就是红，不靠自觉。

## 它解决什么

代理改代码有三个反复出现的失效模式：

1. **越层**。UI 直接调数据库、feature 目录出现原生 HTML、业务模块散落 SQL。代码能跑，边界没了。
2. **假绿**。测试通过，但断言早被改成永远成立。没人知道上一次红是什么时候。
3. **债务复利**。存量违规不敢改，新增违规继续加，几周后规则形同虚设。

| 机制 | 命令 | 做法 |
|---|---|---|
| 分层守卫 | `ritsu check` | `.ritsu/architecture.yaml` 声明层级顺序，反向依赖按真实 import 行定位报错 |
| 证据链 | `ritsu verify` | 只校验证据是否完整、是否红改绿，不替用户跑测试 |
| 违规棘轮 | `ritsu check --strict` | 存量写入基线，新增即红；认债必须显式 `--accept-debt` 并留理由 |

## 安装

需要 Go 1.26+。零第三方依赖。

```bash
go build -o ritsu ./cmd/ritsu
```

## 快速开始

```bash
ritsu init --stack ts --yes     # 新仓库
ritsu adopt --stack go --yes    # 纳管既有仓库，不改动现有代码
ritsu check                     # 静态检查
ritsu check --format agent      # 输出为代理可读结构
ritsu plan --goal "接入支付"     # 执行计划与接力卡
ritsu verify                    # 证据链校验
```

## 子命令

| 命令 | 作用 |
|---|---|
| `init` | 初始化新仓库，生成模板与架构声明 |
| `adopt` | 纳管既有仓库，识别现状后补治理文件 |
| `sync` | 增量同步上游模板，保留本地定制 |
| `check` | 静态检查：分层、规则、边界、自证 |
| `verify` | 红绿证据链校验 |
| `plan` | 生成执行计划与接力卡 |
| `handoff` | 更新接力卡字段 |
| `schema` | 参数契约自省 |
| `version` | 版本 |

支持的栈：`ts` `js` `py` `go` `rust` `java`。

## 输出契约

stdout 只出 JSON（含 `schemaVersion`），stderr 只出日志。

| 退出码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 运行故障 |
| 2 | 参数或契约错误 |

错误码具备明确语义，例如 `ENGINE_MISSING`、`ENGINE_TIMEOUT`、`CHECK_VIOLATIONS`、`VERIFY_REDGREEN`、`SYNC_CONFLICT`、`STATE_NOT_CLEAN`、`USAGE_SCOPE_OUTSIDE`。

## 架构

15 层，单向依赖，声明即约束：

```
contract → fsx → execx → clockx → logx → config → engine
        → ownership → ratchet → lint → plan → template
        → vcs → app → surface
```

| 层 | 职责 |
|---|---|
| `contract` | 错误类型、输出信封、端口定义 |
| `fsx` | 文件系统：原子写、目录遍历、符号链接逃逸拦截 |
| `execx` | 外部进程：超时优先判定、argv 安全传递 |
| `clockx` | 时钟注入，便于测试 |
| `logx` | 日志，只写 stderr |
| `config` | 受限 YAML 解析、glob 匹配、answers 读取 |
| `engine` | 外部引擎协议 |
| `ownership` | 签名清单，损坏即 fail-closed |
| `ratchet` | 违规棘轮，指纹不含行号 |
| `lint` | 规则引擎、依赖图、SARIF 诊断 |
| `plan` | 执行计划与接力卡读写 |
| `template` | 脚手架模板生成 |
| `vcs` | git 操作，拒绝破坏性命令 |
| `app` | 业务用例，全部能力在此 |
| `surface` | CLI 适配，只做解析与分发 |

## 内置规则

10 条，每条都有对应的违规样本自证（`.ritsu/rule-tests/`）：

| 规则 | 级别 | 拦截 |
|---|---|---|
| `no-native-html` | error | feature 目录出现原生 HTML |
| `no-css-in-features` | error | feature 目录内嵌 CSS |
| `no-arbitrary-color` | error | 硬编码色值，绕过设计 Token |
| `sql-outside-db` | error | db 层之外出现 SQL |
| `no-unsafe-cast` | error | 不安全类型断言 |
| `no-bare-id-schema` | error | schema 用裸 id 而非强类型 |
| `no-raw-transport` | error | 绕过传输层直接裸调 |
| `no-console-log` | warning | 遗留 console.log |
| `cap-file-write` | error | 越权写文件 |
| `cap-process-spawn` | error | 越权起进程 |

规则样本防「空集假绿」：规则若因写错而永不命中，样本会先红。

## 测试

端到端测试先于实现存在。13 个包、121 条登记用例，登记表与代码用例双向核对——「文档写 121 条、实际 40 条」这类债会被机械检出。

```bash
go test ./tests/e2e/... -count=1 -p 1
```

| 族 | 条数 | 覆盖 |
|---|---|---|
| IO | 16 | 入口契约、输出信封、退出码、参数校验 |
| INIT | 17 | 模板生成、行数约束、CODEOWNERS 分层 |
| ADOPT | 10 | 既有仓库纳管、多语言识别 |
| SYNC | 10 | 增量同步、本地定制保留、冲突检测 |
| PLAN | 7 | 计划生成、接力卡字段、格式 |
| CHECK | 18 | 分层、规则、strict 升级、字段裁剪 |
| VERIFY | 15 | 证据链、红改绿、密钥脱敏、只读约束 |
| JOURNEY | 3 | 跨命令长链路 |
| SELF | 7 | 自举：仓库对自己跑 check |
| FAULT | 8 | 符号链接逃逸、git 非零退出、引擎超时、并发写 |
| PLAT | 6 | 长路径、argv 引号、大小写归一、生成物换行 |
| PERF | 4 | 5 万文件扫描门槛 |

`-p 1` 是必要的：perf 族生成 5 万文件，并行执行会在 Windows 上耗尽提交内存。

## 自举

这个仓库自己遵守自己的规则。`.ritsu/architecture.yaml` 描述的就是本仓库的 15 层，`ritsu check` 对自己跑零违规。

治理工具管不住自己，就没有说服力。

## 不在范围内

- **不跑业务测试**。`verify` 只校验证据是否齐、是否红改绿；测试由项目自己写、自己跑。
- **不替代 CI**。`check` 是静态检查，可在本地、提交钩子或 CI 运行，但不做构建。
- **不生成业务代码**。`init` 产出的是骨架与规范，不是可运行业务系统。

---

# Ritsu (English)

A governance scaffold for AI coding agents. It installs mechanically enforceable boundaries into a repository: layered dependencies, architecture guards, evidence chains, and violation ratchets.

Humans maintain constraints by memory; agents maintain them by prompt. Both drift. Ritsu turns constraints into something `ritsu check` can adjudicate — fail means red, no self-reporting required.

## What it addresses

Three recurring failure modes when agents edit code:

1. **Layer erosion.** UI calls the database directly, raw HTML appears in a feature directory, SQL scatters across business modules. The code runs; the boundary is gone.
2. **False green.** Tests pass because the assertion was quietly relaxed to always hold. Nobody knows when the last red was.
3. **Compounding debt.** Existing violations are left alone, new ones keep landing, and within weeks the rules are decorative.

| Mechanism | Command | Approach |
|---|---|---|
| Layer guard | `ritsu check` | `.ritsu/architecture.yaml` declares layer order; reverse dependencies are reported at the real import line |
| Evidence chain | `ritsu verify` | Validates that evidence exists and that red became green — it does not run your tests |
| Violation ratchet | `ritsu check --strict` | Existing violations go to a baseline; new ones fail; accepting debt requires explicit `--accept-debt` with a reason |

## Install

Requires Go 1.26+. No third-party dependencies.

```bash
go build -o ritsu ./cmd/ritsu
```

## Quick start

```bash
ritsu init --stack ts --yes     # new repository
ritsu adopt --stack go --yes    # adopt an existing repo without touching its code
ritsu check                     # static check
ritsu check --format agent      # agent-readable output
ritsu plan --goal "add payments"
ritsu verify                    # evidence-chain validation
```

## Commands

| Command | Purpose |
|---|---|
| `init` | Initialize a new repository with templates and an architecture declaration |
| `adopt` | Adopt an existing repository, detect its state, add governance files |
| `sync` | Incrementally sync upstream templates, preserving local customization |
| `check` | Static check: layers, rules, boundaries, self-proof |
| `verify` | Red/green evidence-chain validation |
| `plan` | Generate execution plans and handoff cards |
| `handoff` | Update handoff card fields |
| `schema` | Parameter contract introspection |
| `version` | Version |

Supported stacks: `ts` `js` `py` `go` `rust` `java`.

## Output contract

stdout carries JSON only (including `schemaVersion`); stderr carries logs only.

| Exit code | Meaning |
|---|---|
| 0 | Success |
| 1 | Runtime failure |
| 2 | Usage or contract error |

Error codes are semantic, e.g. `ENGINE_MISSING`, `ENGINE_TIMEOUT`, `CHECK_VIOLATIONS`, `VERIFY_REDGREEN`, `SYNC_CONFLICT`, `STATE_NOT_CLEAN`, `USAGE_SCOPE_OUTSIDE`.

## Architecture

Fifteen layers, strictly one-directional. The declaration is the constraint:

```
contract → fsx → execx → clockx → logx → config → engine
        → ownership → ratchet → lint → plan → template
        → vcs → app → surface
```

| Layer | Responsibility |
|---|---|
| `contract` | Error types, output envelope, port definitions |
| `fsx` | Filesystem: atomic writes, tree walking, symlink escape rejection |
| `execx` | External processes: timeout-first verdicts, safe argv passing |
| `clockx` | Injectable clock for testability |
| `logx` | Logging, stderr only |
| `config` | Restricted YAML parsing, glob matching, answers loading |
| `engine` | External engine protocol |
| `ownership` | Signature manifest, fail-closed on corruption |
| `ratchet` | Violation ratchet, fingerprints exclude line numbers |
| `lint` | Rule engine, dependency graph, SARIF diagnostics |
| `plan` | Execution plan and handoff card I/O |
| `template` | Scaffold template generation |
| `vcs` | git operations, destructive commands refused |
| `app` | Use cases; all capabilities live here |
| `surface` | CLI adapter: parsing and dispatch only |

## Built-in rules

Ten rules, each backed by a violation sample under `.ritsu/rule-tests/`:

| Rule | Severity | Blocks |
|---|---|---|
| `no-native-html` | error | Raw HTML inside a feature directory |
| `no-css-in-features` | error | Inline CSS inside a feature directory |
| `no-arbitrary-color` | error | Hardcoded colors bypassing design tokens |
| `sql-outside-db` | error | SQL outside the db layer |
| `no-unsafe-cast` | error | Unsafe type assertions |
| `no-bare-id-schema` | error | Bare ids instead of strong types in schemas |
| `no-raw-transport` | error | Bypassing the transport layer |
| `no-console-log` | warning | Leftover console.log |
| `cap-file-write` | error | Out-of-scope file writes |
| `cap-process-spawn` | error | Out-of-scope process spawning |

Samples defend against "vacuous green": if a rule is miswritten and never matches, its sample goes red first.

## Tests

End-to-end tests predate the implementation. Thirteen packages, 121 registered cases, cross-checked in both directions between the registry and the code — "registration says 121, reality has 40" is detected mechanically, not by inspection.

```bash
go test ./tests/e2e/... -count=1 -p 1
```

| Family | Cases | Coverage |
|---|---|---|
| IO | 16 | Entry contract, output envelope, exit codes, argument validation |
| INIT | 17 | Template generation, line-count limits, CODEOWNERS layering |
| ADOPT | 10 | Adopting existing repositories, multi-language detection |
| SYNC | 10 | Incremental sync, local customization preserved, conflict detection |
| PLAN | 7 | Plan generation, handoff fields, formatting |
| CHECK | 18 | Layers, rules, strict promotion, field trimming |
| VERIFY | 15 | Evidence chain, red-to-green, secret redaction, read-only guarantees |
| JOURNEY | 3 | Long cross-command paths |
| SELF | 7 | Self-hosting: the repo checks itself |
| FAULT | 8 | Symlink escapes, non-zero git exits, engine timeouts, concurrent writes |
| PLAT | 6 | Long paths, argv quoting, case normalization, generated-file line endings |
| PERF | 4 | 50k-file scan threshold |

`-p 1` is required: the perf family generates 50,000 files, and parallel execution exhausts the Windows commit charge.

## Self-hosting

This repository obeys its own rules. `.ritsu/architecture.yaml` describes its own fifteen layers, and `ritsu check` reports zero violations against itself.

A governance tool that cannot govern itself has no argument.

## Out of scope

- **It does not run your tests.** `verify` checks that evidence is present and that red became green; the tests themselves are yours to write and run.
- **It does not replace CI.** `check` is a static check that can run locally, in a hook, or in CI — it does not build.
- **It does not generate business code.** `init` produces a skeleton and conventions, not a working application.

## License

Not yet chosen.
