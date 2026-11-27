// Package vcs 是 git 的唯一入口。破坏性命令一律拒绝。
package vcs

import (
	"strings"
	"time"

	"ritsu/internal/contract"
)

// Git 实现 contract.VCS。
type Git struct{ exec contract.Executor }

// New 注入执行器端口。
func New(ex contract.Executor) *Git { return &Git{exec: ex} }

var destructive = [][]string{
	{"reset", "--hard"},
	{"clean", "-f"},
	{"clean", "-fd"},
	{"push", "--force"},
	{"push", "-f"},
	{"checkout", "--", "."},
}

// run 统一入口：先拒绝破坏性命令，再把非零退出转成明确错误（绝不静默吞掉）。
func (g *Git) run(root string, args ...string) (contract.Result, error) {
	for _, d := range destructive {
		if len(args) >= len(d) && strings.Join(args[:len(d)], " ") == strings.Join(d, " ") {
			return contract.Result{}, contract.NewErr(contract.CodeUsage,
				"拒绝执行破坏性 git 命令: git %s", strings.Join(args, " "))
		}
	}
	res, err := g.exec.Run(root, append([]string{"git"}, args...))
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		detail := strings.TrimSpace(string(res.Stderr))
		if detail == "" {
			detail = strings.TrimSpace(string(res.Stdout))
		}
		return res, contract.NewErr(contract.CodeInternal,
			"git %s 失败（exit %d）: %s", strings.Join(args, " "), res.ExitCode, detail)
	}
	return res, nil
}

// IsClean 判断工作树是否无未提交改动。
func (g *Git) IsClean(root string) (bool, error) {
	res, err := g.run(root, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(res.Stdout)) == "", nil
}

// Init 初始化仓库。
func (g *Git) Init(root string) error {
	_, err := g.run(root, "init", "-q", "-b", "main")
	return err
}

// Add 暂存指定文件。
func (g *Git) Add(root string, rels ...string) error {
	if len(rels) == 0 {
		return contract.NewErr(contract.CodeUsage, "add 必须显式指定文件")
	}
	_, err := g.run(root, append([]string{"add", "--"}, rels...)...)
	return err
}

// AddRetry 暂存文件；index.lock 冲突时短暂重试，仍失败则明确报错而非静默。
func (g *Git) AddRetry(root string, rels ...string) error {
	if len(rels) == 0 {
		return nil
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		err := g.Add(root, rels...)
		if err == nil {
			return nil
		}
		last = err
		if !strings.Contains(err.Error(), "index.lock") {
			// 不是并发锁，重试无意义，直接如实上报
			return err
		}
		time.Sleep(120 * time.Millisecond)
	}
	return contract.NewErr(contract.CodeInternal,
		"git 索引被占用（.git/index.lock），暂存失败: %v", last).
		WithHint("确认没有其它 git 进程在运行，删除 .git/index.lock 后重试")
}

// Commit 提交。
func (g *Git) Commit(root, msg string) error {
	_, err := g.run(root, "commit", "-q", "-m", msg)
	return err
}

// Head 返回当前提交。
func (g *Git) Head(root string) (string, error) {
	res, err := g.run(root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// IsRepo 判断是否已在 git 仓库内。
func (g *Git) IsRepo(root string) bool {
	res, err := g.run(root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(res.Stdout)) == "true"
}
