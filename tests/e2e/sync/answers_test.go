package sync_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// answers.yml 缺 stack 时必须退出 2，不得静默默认成 ts。
// 静默兜底会让 sync 渲染出与项目实际栈不符的骨架，属于比失败更糟的"成功"。
func TestSyncAnswersMissingStackRejected(t *testing.T) {
	r := managed(t)
	r.Write(".ritsu/answers.yml", "team_size: small\norchestrator: turborepo\ntemplate_version: 1\n")
	r.CommitAll("chore: 破坏 answers")

	res := r.Run("sync")
	fixture.AssertExit(t, res, 2)
	if !strings.Contains(fixture.Str(t, res.Envelope, "error.message"), "stack") {
		t.Fatalf("应指明缺 stack: %s", res.Stdout)
	}
}

// answers.yml 为空文件时必须退出 2，不得当成"无定制"继续合并。
func TestSyncAnswersEmptyRejected(t *testing.T) {
	r := managed(t)
	r.Write(".ritsu/answers.yml", "\n\n")
	r.CommitAll("chore: 清空 answers")

	res := r.Run("sync")
	fixture.AssertExit(t, res, 2)
	if code := fixture.Str(t, res.Envelope, "error.code"); code == "" {
		t.Fatalf("失败必须带 error.code")
	}
}
