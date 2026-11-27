// Package contract 定义全部跨层类型：错误码、JSON 信封、端口接口。
// 本包不得 import 任何 internal 包，否则依赖倒置失效。
package contract

import (
	"errors"
	"fmt"
)

// SchemaVersion 是信封协议版本，所有 stdout 输出的唯一格式来源。
const SchemaVersion = "1.0"

// 退出码三态。全仓只允许这三个值。
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Code 是稳定错误码。对外契约的一部分，不得随实现改名。
type Code string

const (
	CodeUsage        Code = "USAGE_INVALID"
	CodeUnknownFlag  Code = "USAGE_UNKNOWN_FLAG"
	CodeSecretFlag   Code = "USAGE_SECRET_FLAG"
	CodeScopeOutside Code = "USAGE_SCOPE_OUTSIDE"
	CodeNotAClean    Code = "STATE_NOT_CLEAN"
	CodeNoBaseline   Code = "STATE_NO_BASELINE"
	CodeViolations   Code = "CHECK_VIOLATIONS"
	CodeEmptyScope   Code = "CHECK_EMPTY_SCOPE"
	CodeGraphEdges   Code = "CHECK_GRAPH_EDGES"
	CodeRedGreen     Code = "VERIFY_REDGREEN"
	CodeConflict     Code = "SYNC_CONFLICT"
	CodeExists       Code = "STATE_ALREADY_MANAGED"
	CodeNotFound     Code = "STATE_NOT_FOUND"
	CodeInternal     Code = "INTERNAL"
	CodeTimeout         Code = "EXEC_TIMEOUT"
	CodeEngineMissing   Code = "ENGINE_MISSING"
	CodeEngineFailed    Code = "ENGINE_FAILED"
	CodeEngineTimeout   Code = "ENGINE_TIMEOUT"
	CodeEngineMalformed Code = "ENGINE_MALFORMED_OUTPUT"
)

// Err 是结构化错误，携带错误码、人类信息与可执行建议。
type Err struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (e *Err) Error() string { return string(e.Code) + ": " + e.Message }

// NewErr 构造结构化错误。
func NewErr(code Code, format string, args ...any) *Err {
	return &Err{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithHint 追加建议。
func (e *Err) WithHint(h string) *Err {
	e.Hint = h
	return e
}

// WithPath 附着路径。
func (e *Err) WithPath(p string) *Err {
	e.Path = p
	return e
}

// As 提取 *Err；非结构化错误归为 INTERNAL。
func As(err error) *Err {
	var e *Err
	if errors.As(err, &e) {
		return e
	}
	return NewErr(CodeInternal, "%v", err)
}

// ExitCodeFor 把错误码映射到三态退出码。这是唯一映射点。
func ExitCodeFor(c Code) int {
	switch c {
	case CodeUsage, CodeUnknownFlag, CodeSecretFlag, CodeScopeOutside, CodeNotFound,
		CodeNotAClean, CodeExists:
		return ExitUsage
	default:
		return ExitFailure
	}
}

// Diagnostic 是归一化后的诊断。引擎差异在此抹平。
type Diagnostic struct {
	Engine    string   `json:"engine"`
	RuleID    string   `json:"rule_id"`
	Severity  string   `json:"severity"` // error | warning
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Message   string   `json:"message"`
	FlowSteps []string `json:"flow_steps,omitempty"`
	FixHint   string   `json:"fix_hint,omitempty"`
}
