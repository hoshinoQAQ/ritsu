// Package execx 是创建子进程的唯一入口。
// 一律 argv 数组直连原生可执行文件：禁 shell 字符串、禁 .bat/.cmd 直调。
package execx

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ritsu/internal/contract"
)

// Exec 实现 contract.Executor。
type Exec struct{ Timeout time.Duration }

// New 构造执行器，默认 120 秒超时。
func New(timeout time.Duration) *Exec {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Exec{Timeout: timeout}
}

// forbidden 是绝对不允许直调的解释器与批处理包装器。
var forbidden = map[string]bool{
	"sh": true, "bash": true, "zsh": true,
	"cmd": true, "cmd.exe": true,
	"powershell": true, "powershell.exe": true,
	"pwsh": true, "pwsh.exe": true,
}

// Run 执行 argv。返回原始字节，不做任何语义解析。
func (e *Exec) Run(dir string, argv []string) (contract.Result, error) {
	return e.RunTimeout(dir, argv, e.Timeout)
}

// RunTimeout 以指定超时执行 argv。超时优先判定，不被子进程退出码掩盖。
func (e *Exec) RunTimeout(dir string, argv []string, timeout time.Duration) (contract.Result, error) {
	if len(argv) == 0 {
		return contract.Result{}, contract.NewErr(contract.CodeUsage, "argv 不能为空")
	}
	exe := strings.ToLower(filepath.Base(argv[0]))
	if forbidden[exe] {
		return contract.Result{}, contract.NewErr(contract.CodeUsage,
			"禁止通过 shell 执行: %s", argv[0]).
			WithHint("用 argv 数组直连原生可执行文件")
	}
	if strings.HasSuffix(exe, ".bat") || strings.HasSuffix(exe, ".cmd") {
		return contract.Result{}, contract.NewErr(contract.CodeUsage,
			"禁止直调批处理包装器: %s", argv[0]).
			WithHint("BatBadBut：Windows 会在底层静默拉起 cmd.exe")
	}
	if timeout <= 0 {
		timeout = e.Timeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := contract.Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err != nil {
		// 超时必须先判：否则会被 ExitError 吞成"退出码非零"
		if ctx.Err() == context.DeadlineExceeded {
			res.ExitCode = -1
			return res, contract.NewErr(contract.CodeTimeout,
				"命令超时（%s）: %s", timeout, argv[0])
		}
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, err
	}
	return res, nil
}
