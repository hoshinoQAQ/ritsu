package config

import (
	"os"
	"path/filepath"
	"strings"

	"ritsu/internal/contract"
)

// AnswersPath 是纳管答案文件的位置，决定模板渲染参数。
const AnswersPath = ".ritsu/answers.yml"

// Answers 是 init/adopt 时确定下来的项目参数。
type Answers struct {
	Stack    []string
	TeamSize string
}

// LoadAnswers 读取并校验答案文件。
// 文件缺失、为空、无法解析、缺 stack 一律返回结构化错误：
// 静默默认一个技术栈会让 sync 悄悄渲染出错误的骨架，比直接失败危险得多。
func LoadAnswers(root string) (*Answers, error) {
	full := filepath.Join(root, filepath.FromSlash(AnswersPath))
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, contract.NewErr(contract.CodeNotFound,
				"缺少 %s，无法确定基准版本", AnswersPath).
				WithHint("先运行 ritsu init 或 ritsu adopt").
				WithPath(AnswersPath)
		}
		return nil, contract.NewErr(contract.CodeInternal,
			"读取 %s 失败: %v", AnswersPath, err).WithPath(AnswersPath)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, contract.NewErr(contract.CodeUsage,
			"%s 是空文件", AnswersPath).WithPath(AnswersPath).
			WithHint("重新运行 ritsu init 覆盖生成")
	}

	m, err := parseYAML(string(data))
	if err != nil {
		return nil, contract.NewErr(contract.CodeUsage,
			"%s 解析失败: %v", AnswersPath, err).WithPath(AnswersPath)
	}

	a := &Answers{}
	switch t := m["stack"].(type) {
	case []string:
		a.Stack = t
	case string:
		if s := strings.TrimSpace(t); s != "" {
			a.Stack = []string{s}
		}
	}
	if len(a.Stack) == 0 {
		return nil, contract.NewErr(contract.CodeUsage,
			"%s 缺 stack 字段", AnswersPath).WithPath(AnswersPath).
			WithHint("补齐 stack: [ts]，或重新运行 ritsu init")
	}
	if ts, ok := m["team_size"].(string); ok {
		a.TeamSize = strings.TrimSpace(ts)
	}
	return a, nil
}
