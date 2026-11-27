// Package engine 是外部检查引擎的适配层。
// 引擎失败、超时、输出畸形一律转成结构化错误，绝不 panic、绝不静默降级。
package engine

import (
	"encoding/json"
	"strings"
	"time"

	"ritsu/internal/contract"
	"ritsu/internal/fsx"
)

// Path 是引擎声明文件。
const Path = ".ritsu/engines.json"

// Spec 是一个外部引擎的启动方式。
type Spec struct {
	ID        string   `json:"id"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	TimeoutMS int      `json:"timeout_ms"`
}

// File 是引擎清单。
type File struct {
	Engines []Spec `json:"engines"`
}

// Load 读取引擎清单。文件不存在视为未配置任何外部引擎。
func Load(files *fsx.FS, root string) (*File, error) {
	f := &File{}
	if !files.Exists(root, Path) {
		return f, nil
	}
	data, err := files.Read(root, Path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, contract.NewErr(contract.CodeUsage,
			"%s 解析失败: %v", Path, err).WithPath(Path)
	}
	for _, s := range f.Engines {
		if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Command) == "" {
			return nil, contract.NewErr(contract.CodeUsage,
				"%s 中每个引擎都必须有 id 与 command", Path).WithPath(Path)
		}
	}
	return f, nil
}

// Run 逐个执行引擎，把输出解析成归一化诊断。
func (f *File) Run(ex contract.Executor, root string) ([]contract.Diagnostic, error) {
	var out []contract.Diagnostic
	for _, s := range f.Engines {
		timeout := time.Duration(s.TimeoutMS) * time.Millisecond
		argv := append([]string{s.Command}, s.Args...)
		res, err := ex.RunTimeout(root, argv, timeout)
		if err != nil {
			e := contract.As(err)
			// 参数/契约类错误原样透传：这是配置问题，不是引擎缺失
			if e.Code == contract.CodeUsage ||
				e.Code == contract.CodeUnknownFlag || e.Code == contract.CodeSecretFlag {
				return nil, e
			}
			// 只认错误码：文案会被改写，类型不会
			if e.Code == contract.CodeTimeout {
				return nil, contract.NewErr(contract.CodeEngineTimeout,
					"引擎 %s 超时（%dms）", s.ID, s.TimeoutMS).
					WithHint("排查引擎是否卡住；调大 timeout_ms 或修复引擎实现")
			}
			return nil, contract.NewErr(contract.CodeEngineMissing,
				"引擎 %s 无法启动: %v", s.ID, err).
				WithHint("安装该引擎并确认 " + s.Command + " 在 PATH 中可执行")
		}
		if res.ExitCode != 0 {
			// 引擎启动了并给出结果，只是判失败：不能报成「没安装」
			return nil, contract.NewErr(contract.CodeEngineFailed,
				"引擎 %s 退出码 %d: %s", s.ID, res.ExitCode,
				strings.TrimSpace(string(res.Stderr))).
				WithHint("按引擎自身提示修复；临时可只跑内置规则: ritsu check --rule <id>")
		}

		var diags []contract.Diagnostic
		if err := json.Unmarshal(res.Stdout, &diags); err != nil {
			snippet := strings.TrimSpace(string(res.Stdout))
			if len(snippet) > 120 {
				snippet = snippet[:120] + "…"
			}
			return nil, contract.NewErr(contract.CodeEngineMalformed,
				"引擎 %s 输出不是合法 JSON 诊断数组: %v", s.ID, err).
				WithHint("引擎必须输出 [{rule_id,severity,file,line,message}]，实际: " + snippet)
		}
		for i := range diags {
			diags[i].Engine = s.ID
			if diags[i].Severity == "" {
				diags[i].Severity = "error"
			}
		}
		out = append(out, diags...)
	}
	return out, nil
}
