// Package template 负责模板合成。它是 init 的唯一内容来源。
// 模板内绝不出现密钥、绝对路径或公司域名。
package template

import (
	"strings"
)

// Options 是合成参数。
type Options struct {
	Stack    []string // ts / python / go
	TeamSize string   // small | large
}

// File 是一个待落盘文件。
type File struct {
	Path    string
	Content string
}

// Orchestrator 按语言构成与团队规模路由工作区编排器。
func Orchestrator(o Options) string {
	langs := map[string]bool{}
	for _, s := range o.Stack {
		langs[strings.ToLower(strings.TrimSpace(s))] = true
	}
	if len(langs) >= 2 {
		return "moon"
	}
	if o.TeamSize == "large" {
		return "nx"
	}
	return "turborepo"
}

// Render 生成全套骨架。调用方负责 Tier 判定与写盘。
func Render(o Options) []File {
	orch := Orchestrator(o)
	files := []File{
		{".gitignore", gitignore},
		{"AGENTS.md", agentsMD},
		{"CODEOWNERS", codeowners},
		{".ritsu/architecture.yaml", architectureYAML},
		{".ritsu/answers.yml", answersYAML(o, orch)},
		{".ritsu/hooks/pre-tool-use.sh", preToolUse},
		{".ritsu/rules/README.md", rulesReadme},
		{".ritsu/rule-tests/README.md", ruleTestsReadme},
		{".github/workflows/ritsu-gate.yml", gateWorkflow},
		{"docs/architecture/layers.md", layersDoc},
		{"docs/quality/README.md", qualityDoc},
		{"docs/exec-plans/active/.gitkeep", ""},
		{"agent/CONTINUITY.md", continuityMD},
		{"agent/brief.md", briefMD},
		{"contracts/.gitkeep", ""},
		{"package.json", packageJSON},
		{"tsconfig.json", tsconfigJSON},
		{"tools/ritsu.mjs", ritsuMJS},
		{"tests/smoke.test.mjs", smokeTestMJS},
		{"src/shared/ids.ts", idsTS},
		{"src/types/index.ts", typesTS},
		{"src/service/index.ts", serviceTS},
		{"src/ui/primitives/Button.tsx", buttonTSX},
		{"src/ui/features/.gitkeep", ""},
		{"src/db/.gitkeep", ""},
		{"src/api/controllers/.gitkeep", ""},
	}
	switch orch {
	case "turborepo":
		files = append(files, File{"turbo.json", turboJSON})
	case "nx":
		files = append(files, File{"nx.json", nxJSON})
	case "moon":
		files = append(files, File{"moon.yml", moonYML})
		files = append(files, File{".tool-versions", toolVersions})
	}
	return files
}

// Tier1Paths 返回需要签名与只读保护的文件。与 Options 无关：Tier1 集合不随栈变化。
func Tier1Paths() []string {
	return []string{
		".ritsu/hooks/pre-tool-use.sh",
		".ritsu/rules/README.md",
		".ritsu/rule-tests/README.md",
		".github/workflows/ritsu-gate.yml",
		"CODEOWNERS",
	}
}

const gitignore = `node_modules/
dist/
.ritsu/logs/
.ritsu/bin/
*.tmp
`

// agentsMD 严格控制在 60–100 行：只做索引与红线，绝不复制正本内容。
var agentsMD = func() string {
	lines := []string{
		"# AGENTS.md",
		"",
		"> 入口索引。正本在 docs/，本文件不复制正本内容。",
		"> 每次会话先读这里，再按链接按需下钻。",
		"",
		"## 技术栈快照",
		"",
		"- TypeScript / Node 20+",
		"- 包管理与工作区见根配置",
		"- 静态检查：ritsu check",
		"",
		"## 确定性命令",
		"",
		"```",
		"ritsu check --format agent",
		"ritsu verify --strict",
		"```",
		"",
		"## 硬红线",
		"",
		"1. 禁止修改 .ritsu/hooks、.ritsu/rules、.ritsu/rule-tests、",
		"   .github/workflows/ritsu-gate.yml、CODEOWNERS。",
		"2. 禁止删除或跳过测试，禁止 --no-verify，禁止放宽断言。",
		"3. 禁止用 as any / @ts-ignore 绕过类型错误。",
		"4. SQL 只能写在 src/db/，接口注册只能写在 src/api/controllers/。",
		"5. 业务 UI 禁止新建样式文件，禁止原生 HTML 交互标签。",
		"6. 禁止硬编码色值，包含 Tailwind 任意值。",
		"7. 实体 ID 必须用 src/shared/ids.ts 的 Branded 类型。",
		"8. 禁止手拼路由路径，必须走强类型路由构造器。",
		"9. 出站请求必须带显式超时。",
		"10. 每条新增规则必须绑定一次真实故障，并带日期。",
		"",
		"## 索引",
		"",
		"- 分层与包拓扑：docs/architecture/layers.md",
		"- 质量与测试现状：docs/quality/README.md",
		"- 执行计划：docs/exec-plans/",
		"- 跨窗口接力：agent/CONTINUITY.md",
		"- 自举声明：.ritsu/architecture.yaml",
		"",
		"## 会话纪律",
		"",
		"- 开工先读 agent/CONTINUITY.md 的八个字段。",
		"- 先写失败的测试并留存红灯日志，再写实现。",
		"- 完成后跑 ritsu verify --strict，不要只跑单测。",
		"- 遇到无法完成的情况，走 Infeasible 上报，不要伪造通过。",
		"",
		"## 上下文预算",
		"",
		"- 本文件不超过 100 行。",
		"- 需要细节时读 docs/，不要在此堆叠。",
		"- 报错信息只保留关键字段与复现命令。",
		"",
		"## 提交纪律",
		"",
		"- 一个自包含改动一个提交。",
		"- 先重构后功能。",
		"- 提交信息 <type>(<scope>): <summary>。",
		"- 验证不过不许提交。",
		"",
		"## 禁止事项（速查）",
		"",
		"- 不碰 Tier 1 文件。",
		"- 不改测试让测试通过。",
		"- 不写裸 SQL。",
		"- 不新增 CSS 文件。",
		"- 不硬编码颜色。",
		"- 不用裸 string 当实体 ID。",
		"- 不手拼 URL 路径。",
		"- 不省略请求超时。",
		"- 不在日志里出现密钥。",
		"- 不留 console.log。",
		"",
	}
	return strings.Join(lines, "\n")
}()

const codeowners = `# 治理层归平台组所有，代理无写权限
/.ritsu/            @platform-team
/.github/workflows/   @platform-team
/CODEOWNERS           @platform-team
/contracts/           @platform-team
# 配置层：改动需双人复核
/AGENTS.md            @platform-team @core-team
/package.json         @core-team
/tsconfig.json        @core-team
# 自由演进区：按目录归属
/src/ui/              @frontend-team
/src/                 @core-team
/docs/                @docs-team
`

const architectureYAML = `order: [types, config, db, service, runtime, ui, api]
paths:
  types: ["src/types/**"]
  config: ["src/config/**"]
  db: ["src/db/**"]
  service: ["src/service/**"]
  runtime: ["src/runtime/**"]
  ui: ["src/ui/**"]
  api: ["src/api/**"]
graph:
  min_edges: 1
`

func answersYAML(o Options, orch string) string {
	return "stack: [" + strings.Join(o.Stack, ", ") + "]\n" +
		"team_size: " + orDefault(o.TeamSize, "small") + "\n" +
		"orchestrator: " + orch + "\n" +
		"template_version: 1\n"
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

const preToolUse = `#!/bin/sh
# Tier 1 只读治理钩子：拦截对测试、规则、CI 的未授权改动
set -eu
for p in "$@"; do
  case "$p" in
    .ritsu/hooks/*|.ritsu/rules/*|.ritsu/rule-tests/*|.github/workflows/*|CODEOWNERS|*_test.go|*.test.ts)
      echo "blocked: $p 属于治理层或测试资产" >&2
      exit 1
      ;;
  esac
done
`

const rulesReadme = `# 规则目录

本目录由 ritsu 编译生成，头部带只读标记，代理禁止改写。

每条规则必须有配套的 rule-tests 正反例；反例跑不红即视为规则失效。
`

const ruleTestsReadme = `# 规则快照

每个 YAML 含 valid 与 invalid 两组样本。CI 先跑快照，再跑全量扫描。
`

const gateWorkflow = `name: ritsu-gate
on: [push, pull_request]
jobs:
  gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: check
        run: ritsu check --strict --format agent
      - name: verify
        run: ritsu verify --strict
`

const layersDoc = `# 分层与包拓扑

单向依赖：types → config → db → service → runtime → ui → api

- 下游只能依赖排在前面的层，禁止反向或跨层直连。
- SQL 只允许写在 src/db/。
- 接口注册只允许写在 src/api/controllers/。
`

const qualityDoc = `# 质量现状

由 CI 生成，不手工填写数字。

- 测试类型：单元 / 属性 / 架构守卫 / 契约 / 端到端
- 门禁：ritsu check --strict
`

// continuityMD 严格八个字段。
const continuityMD = `# CONTINUITY

## 当前目标

## 验收标准

## 首选必读文档

## 已作决策

## 当前进度

## 阻塞项

## 下一步行动

## 仍需完成的验证
`

const briefMD = `本仓库由 ritsu 治理：规则在 .ritsu/，正本在 docs/，入口索引是 AGENTS.md。
`

// packageJSON：test 始终是项目自己的测试入口，ritsu 作为 pretest 闸门前置执行。
// 这样 agent 只需跑 npm test：项目测试照跑，违反治理红线时在 pretest 阶段就被拦住。
const packageJSON = `{
  "name": "app",
  "private": true,
  "version": "0.0.0",
  "scripts": {
    "pretest": "node tools/ritsu.mjs check --format agent",
    "test": "node --test",
    "check": "node tools/ritsu.mjs check --format agent",
    "verify": "node tools/ritsu.mjs verify --strict"
  },
  "devDependencies": {
    "typescript": "^5.6.0"
  }
}
`

// smokeTestMJS 是项目自带的第一条测试：验证骨架产物齐备。
// 它证明 npm test 跑的不只是 ritsu，项目自己也有测试入口。
const smokeTestMJS = `import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";

const root = new URL("../", import.meta.url);

test("骨架产物齐备", () => {
  for (const p of ["AGENTS.md", ".ritsu/architecture.yaml", ".ritsu/answers.yml", "CODEOWNERS"]) {
    assert.ok(existsSync(new URL(p, root)), p + " 缺失");
  }
});
`
// ritsuMJS 是跨平台包装器：项目内自带二进制，不依赖 PATH。
const ritsuMJS = `#!/usr/bin/env node
// 定位并调用项目内置的 ritsu。agent 只需跑 npm test，无需知道 ritsu 存在。
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const name = process.platform === "win32" ? "ritsu.exe" : "ritsu";
const bin = process.env.RITSU_BIN || join(root, ".ritsu", "bin", name);

if (!existsSync(bin)) {
  console.error(
    "ritsu 二进制缺失：" + bin + "\n" +
    "在项目根目录运行一次 ritsu init，或从发布包复制到 .ritsu/bin/"
  );
  process.exit(2);
}

const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit", cwd: root });
process.exit(r.status === null ? 1 : r.status);
`

const tsconfigJSON = `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noEmit": true,
    "baseUrl": ".",
    "paths": { "@/*": ["src/*"] }
  },
  "include": ["src"]
}
`

const idsTS = `// 实体 ID：Branded 类型，禁止用裸 string 传递
type Brand<T, B> = T & { readonly __brand: B };

/** 以 run_ 为前缀的实体 ID */
export type RunId = Brand<string, "RunId">;
/** 以 ses_ 为前缀的实体 ID */
export type SessionId = Brand<string, "SessionId">;

const runPrefix = "run_";
const sesPrefix = "ses_";

export function asRunId(v: string): RunId {
  if (!v.startsWith(runPrefix)) {
    throw new Error('Expected ID starting with "run_", got "' + v + '"');
  }
  return v as RunId;
}

export function asSessionId(v: string): SessionId {
  if (!v.startsWith(sesPrefix)) {
    throw new Error('Expected ID starting with "ses_", got "' + v + '"');
  }
  return v as SessionId;
}
`

const typesTS = `export interface RunSummary {
  runId: string;
  title: string;
  status: "idle" | "running" | "done";
}
`

const serviceTS = `import type { RunSummary } from "@/types";

export function summarize(runId: string, title: string): RunSummary {
  return { runId, title, status: "idle" };
}
`

const buttonTSX = `import type { ReactNode } from "react";

export function Button(props: { children: ReactNode; onClick?: () => void }) {
  return <button onClick={props.onClick}>{props.children}</button>;
}
`

const turboJSON = `{
  "pipeline": {
    "check": { "outputs": [] },
    "verify": { "outputs": [] }
  }
}
`

const nxJSON = `{
  "defaultBase": "main",
  "targetDefaults": {
    "check": { "cache": true }
  }
}
`

const moonYML = `$schema: "https://moonrepo.dev/schemas/workspace.json"
version: 1
projects:
  - "."
`

// toolVersions 锁定多栈运行时版本，保证三端可复现。
const toolVersions = `nodejs 22.11.0
python 3.12.7
golang 1.23.4
`
