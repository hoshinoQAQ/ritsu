package lint

import (
	"strings"
)

// Rules 是内置禁例集。每条都必须能配一个违规样本证明它会红。
func Rules() []Rule {
	return []Rule{
		{
			ID: "no-native-html", Engine: "ritsu", Severity: "error",
			Exts:  []string{".tsx"},
			Scope: []string{"src/ui/features/**"},
			Check: func(rel string, lines []string) []Hit {
				return scanTags(lines, []string{"<button", "<input", "<select", "<textarea", "<a "},
					"业务模块禁止直接使用原生 HTML 交互组件",
					"改用 @/ui/primitives 下的封装组件")
			},
		},
		{
			ID: "no-css-in-features", Engine: "ritsu", Severity: "error",
			Exts:  []string{".css", ".scss", ".less"},
			Scope: []string{"src/ui/features/**"},
			Check: func(rel string, lines []string) []Hit {
				return []Hit{{Line: 1,
					Message: "业务模块禁止新建样式文件，一律使用 design token 与既有组件",
					Fix:     "删除该文件，改用现有 token 与 tailwind 类"}}
			},
		},
		{
			ID: "no-arbitrary-color", Engine: "ritsu", Severity: "error",
			Exts: []string{".ts", ".tsx", ".css"},
			Check: func(rel string, lines []string) []Hit {
				var hits []Hit
				for i, line := range lines {
					if hasArbitraryHex(line) {
						hits = append(hits, Hit{
							Line:    i + 1,
							Message: "禁止 Tailwind 任意值硬编码色值",
							Fix:     "改用 var(--color-*) 或主题 token",
						})
					}
				}
				return hits
			},
		},
		{
			ID: "sql-outside-db", Engine: "ritsu", Severity: "error",
			Exts:    []string{".ts", ".tsx", ".js"},
			Scope:   []string{"src/**"},
			Exclude: []string{"src/db/**"},
			Check: func(rel string, lines []string) []Hit {
				hits := scanTags(lines, []string{"store.sqlite.", "db.prepare(", "better-sqlite3"},
					"SQL 句柄与数据库驱动只允许出现在 src/db/",
					"通过仓储接口调用，不要在业务层拼 SQL")
				for i, line := range lines {
					if hasRawSQL(line) {
						hits = append(hits, Hit{Line: i + 1,
							Message: "SQL 文本只允许出现在 src/db/",
							Fix:     "把查询下沉到 src/db 的仓储实现"})
					}
				}
				return hits
			},
		},
		{
			ID: "no-unsafe-cast", Engine: "ritsu", Severity: "error",
			Exts:  []string{".ts", ".tsx"},
			Scope: []string{"src/**"},
			Check: func(rel string, lines []string) []Hit {
				return scanTags(lines, []string{"as any", "as unknown as", "@ts-ignore", "@ts-expect-error"},
					"禁止用类型断言绕过契约校验",
					"修正确的类型，不要强转")
			},
		},
		{
			ID: "no-bare-id-schema", Engine: "ritsu", Severity: "error",
			Exts: []string{".ts", ".tsx"},
			Check: func(rel string, lines []string) []Hit {
				var hits []Hit
				for i, line := range lines {
					if isBareIDSchema(line) {
						hits = append(hits, Hit{
							Line:    i + 1,
							Message: "实体 ID 字段必须是 Branded 类型，不能用裸 z.string()",
							Fix:     "改为引用 src/shared/ids.ts 中的实体 ID Schema",
						})
					}
				}
				return hits
			},
		},
		{
			ID: "no-raw-transport", Engine: "ritsu", Severity: "error",
			Exts:  []string{".ts", ".tsx"},
			Scope: []string{"src/ui/**"},
			Check: func(rel string, lines []string) []Hit {
				return scanTags(lines, []string{"fetch('/api", `fetch("/api`, "ipc.invoke("},
					"禁止手写裸网络/进程调用",
					"只能调用 api-registry 导出的强类型客户端")
			},
		},
		{
			ID: "no-console-log", Engine: "ritsu", Severity: "warning",
			Exts: []string{".ts", ".tsx"},
			Check: func(rel string, lines []string) []Hit {
				return scanTags(lines, []string{"console.log("},
					"生产代码不应保留 console.log",
					"改用结构化日志")
			},
		},
		{
			// 自举能力锁：写盘只允许在 fsx
			ID: "cap-file-write", Engine: "ritsu", Severity: "error",
			Exts: []string{".go"},
			Scope: []string{"internal/**", "cmd/**"},
			// 规则目录本身必须写出被禁符号，否则无法声明禁例
			Exclude: []string{"internal/fsx/**", "internal/lint/rules.go", "**/*_test.go"},
			Check: func(rel string, lines []string) []Hit {
				return scanTags(lines, []string{"os.WriteFile(", "os.Remove(", "os.Rename(", "os.MkdirAll("},
					"写盘能力独占 internal/fsx",
					"通过 contract.FileSystem 端口写入")
			},
		},
		{
			// 自举能力锁：进程创建只允许在 execx
			ID: "cap-process-spawn", Engine: "ritsu", Severity: "error",
			Exts: []string{".go"},
			Scope: []string{"internal/**", "cmd/**"},
			// 规则目录本身必须写出被禁符号，否则无法声明禁例
			Exclude: []string{"internal/execx/**", "internal/lint/rules.go", "**/*_test.go"},
			Check: func(rel string, lines []string) []Hit {
				return scanTags(lines, []string{"exec.Command(", "exec.CommandContext("},
					"进程创建能力独占 internal/execx",
					"通过 contract.Executor 端口执行")
			},
		},
	}
}

func scanTags(lines []string, tags []string, msg, fix string) []Hit {
	var hits []Hit
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		for _, tag := range tags {
			if strings.Contains(line, tag) {
				hits = append(hits, Hit{Line: i + 1, Message: msg, Fix: fix})
				break
			}
		}
	}
	return hits
}

// hasArbitraryHex 识别 Tailwind 任意值里的硬编码色值，如 bg-[#330745]。
func hasArbitraryHex(line string) bool {
	for i := 0; i+1 < len(line); i++ {
		if line[i] == '[' && line[i+1] == '#' {
			j := i + 2
			n := 0
			for j < len(line) && isHex(line[j]) {
				j++
				n++
			}
			if (n == 3 || n == 6 || n == 8) && j < len(line) && line[j] == ']' {
				return true
			}
		}
	}
	return false
}

// hasRawSQL 识别源码里内联的 SQL 文本（SELECT ... FROM 形态）。
func hasRawSQL(line string) bool {
	up := strings.ToUpper(line)
	return strings.Contains(up, "SELECT ") && strings.Contains(up, " FROM ")
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// isBareIDSchema 识别 `xxxId: z.string()` 这类裸 ID 声明。
func isBareIDSchema(line string) bool {
	idx := strings.Index(line, "z.string()")
	if idx < 0 {
		return false
	}
	head := line[:idx]
	colon := strings.LastIndex(head, ":")
	if colon < 0 {
		return false
	}
	key := strings.TrimSpace(head[:colon])
	key = strings.TrimSuffix(key, "?")
	return key == "id" || strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "_id")
}
