package sync_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// SYNC-006 --skip-answered 在 stdin 关闭下也能完成。
func TestSYNC006SkipAnsweredNoInteraction(t *testing.T) {
	r := managed(t)
	r.Write("tsconfig.json", r.Read("tsconfig.json")+"\n// local\n")
	r.CommitAll("chore: local tweak")

	res := r.Run("sync", "--skip-answered")
	fixture.AssertExit(t, res, 0)
	if fixture.Bool(t, res.Envelope, "data.dry_run") {
		t.Fatalf("SYNC-006 不应是 dry-run")
	}
}

// SYNC-009 上游删掉某个 Tier2 文件时不得静默删除本地文件。
func TestSYNC009UpstreamDeletionKeepsLocal(t *testing.T) {
	r := managed(t)
	// 本地额外保留了上游已不再生成的文件
	const extra = "stylelint.config.js"
	r.Write(extra, "module.exports = {};\n")
	r.CommitAll("chore: keep local config")

	res := r.Run("sync")
	fixture.AssertExit(t, res, 0)

	if !r.Exists(extra) {
		t.Fatalf("SYNC-009 本地文件被静默删除")
	}
}

// SYNC-010 上游改 Tier1 且本地也改过：覆盖前必须把原内容留档，可追溯。
func TestSYNC010Tier1OverwriteKeepsOriginal(t *testing.T) {
	r := managed(t)
	const hook = ".ritsu/hooks/pre-tool-use.sh"
	const original = "# 本地改动，必须可追溯\n"
	r.Write(hook, original)
	r.CommitAll("chore: local hook edit")

	res := r.Run("sync")
	fixture.AssertExit(t, res, 0)

	backup := ".ritsu/backup/" + hook
	if !r.Exists(backup) {
		t.Fatalf("SYNC-010 Tier1 被覆盖但原内容未留档")
	}
	if !strings.Contains(r.Read(backup), "本地改动") {
		t.Fatalf("SYNC-010 留档内容不正确: %q", r.Read(backup))
	}
	if !strings.Contains(r.Read(hook), "read-only: managed by ritsu") {
		t.Fatalf("SYNC-010 Tier1 未被上游覆盖")
	}
}
