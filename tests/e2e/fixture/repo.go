package fixture

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Repo 是一条 E2E 用例独占的临时仓库。每条用例一个，避免相互污染。
type Repo struct {
	t    *testing.T
	Dir  string
	Home string
}

// Fixture 名。与设计文档 3.1 节一致。
const (
	Empty       = "empty"
	LegacyTS    = "legacy-ts"
	ManagedV1   = "managed-v1"
	Dirty       = "dirty"
)

// NewRepo 建一个已 git init 并首次提交的临时仓库。
func NewRepo(t *testing.T, kind string) *Repo {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()
	r := &Repo{t: t, Dir: dir, Home: home}

	switch kind {
	case Empty:
		// 什么都不放
	case LegacyTS:
		r.write("package.json", `{"name":"legacy","version":"1.0.0"}`)
		r.write("src/db/store.ts", "export const raw = 'SELECT * FROM t';\n")
		r.write("src/ui/features/card.tsx", "export const Card = () => <button>ok</button>;\n")
		r.write("src/ui/features/style.css", ".a{color:#330745}\n")
		r.write("src/service/user.ts", "export const get = (id: any) => id;\n")
		r.write("tsconfig.json", `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["src/*"]}}}`)
	case ManagedV1:
		r.write("README.md", "# managed\n")
	}

	r.gitInit()
	if kind == Dirty {
		// Dirty 必须在首次提交之后写入，否则 gitInit 把它一并提交掉，「脏树」名不副实
		r.write("README.md", "# dirty\n")
	}
	return r
}

// gitInit 固定作者与时间戳，保证提交 hash 稳定可比。
func (r *Repo) gitInit() {
	r.run("git", "init", "-q", "-b", "main")
	r.run("git", "config", "user.email", "e2e@example.com")
	r.run("git", "config", "user.name", "e2e")
	r.run("git", "config", "commit.gpgsign", "false")
	r.run("git", "config", "core.autocrlf", "false")
	r.run("git", "add", "--all", ".")
	r.commit("chore: init")
	if out := r.GitOut("status", "--porcelain"); out != "" {
		r.t.Fatalf("夹具初始化后工作树仍不干净（%s）:\n%s", r.Dir, out)
	}
}

func (r *Repo) commit(msg string) {
	cmd := r.cmd("git", "commit", "-q", "-m", msg, "--allow-empty")
	cmd.Env = append(cmd.Env,
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git commit 失败: %v\n%s", err, out)
	}
}

// CommitAll 提交当前全部改动，供需要干净工作树的用例使用。
func (r *Repo) CommitAll(msg string) {
	r.run("git", "add", "--all", ".")
	r.commit(msg)
}

// Write 写文件并自动建父目录。
func (r *Repo) Write(rel, content string) { r.write(rel, content) }

func (r *Repo) write(rel, content string) {
	full := filepath.Join(r.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// Kill 启动 ritsu 并在 ms 毫秒后强杀进程，模拟运行中途断电/中断。
func (r *Repo) Kill(ms int, args ...string) (code int, done bool) {
	cmd := exec.Command(Binary(r.t), args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(),
		"RITSU_HOME="+r.Home,
		"NO_COLOR=1",
		"TERM=dumb",
		"RITSU_DEBUG_SLOW_MS=400",
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		r.t.Fatalf("启动失败: %v", err)
	}
	timer := time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
		_ = cmd.Process.Kill()
	})
	defer timer.Stop()
	err := cmd.Wait()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), true
	}
	return 0, err == nil
}

// StrayTemps 返回仓库内残留的原子写临时文件。
func (r *Repo) StrayTemps() []string {
	var out []string
	_ = filepath.Walk(r.Dir, func(p string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".ritsu-tmp") {
			rel, _ := filepath.Rel(r.Dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

// Symlink 在仓库内建一个符号链接。
func (r *Repo) Symlink(target, rel string) {
	full := filepath.Join(r.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	_ = os.Remove(full)
	if err := os.Symlink(target, full); err != nil {
		r.t.Skipf("当前环境不支持符号链接: %v", err)
	}
}

// RemovePath 删除仓库内路径，用于构造磁盘异常。
func (r *Repo) RemovePath(rel string) {
	if err := os.RemoveAll(filepath.Join(r.Dir, filepath.FromSlash(rel))); err != nil {
		r.t.Fatal(err)
	}
}

// ReadOnly 把文件设为只读，用于构造权限拒绝。
func (r *Repo) ReadOnly(rel string) {
	full := filepath.Join(r.Dir, filepath.FromSlash(rel))
	if err := os.Chmod(full, 0o444); err != nil {
		r.t.Fatal(err)
	}
}

// Exec 在仓库目录下运行任意命令，返回合并输出。用于真跑外部工具。
func (r *Repo) Exec(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = r.Dir
	out, err := c.CombinedOutput()
	return string(out), err
}

// itoa 避免在夹具里引入 strconv 之外的依赖。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// RunDuring 启动命令后在第 delayMs 毫秒执行 mutate，用于验证「运行中外部改动」的快照语义。
// slowMs 注入写盘/配置读取后的停顿，让窗口稳定可命中。
func (r *Repo) RunDuring(delayMs, slowMs int, mutate func(), args ...string) Result {
	r.t.Helper()
	extra := []string{}
	if slowMs > 0 {
		extra = append(extra, "RITSU_DEBUG_SLOW_MS="+itoa(slowMs))
	}
	done := make(chan Result, 1)
	go func() {
		res := executeEnv(r.t, r.Dir, r.Home, extra, args)
		checkInvariants(r.t, &res, args)
		done <- res
	}()
	time.Sleep(time.Duration(delayMs) * time.Millisecond)
	mutate()
	return <-done
}

// RunWithEnv 带额外环境变量执行 ritsu。
func (r *Repo) RunWithEnv(extra []string, args ...string) Result {
	r.t.Helper()
	res := executeEnv(r.t, r.Dir, r.Home, extra, args)
	checkInvariants(r.t, &res, args)
	return res
}

// Mkdir 建一个空目录，用于构造「空作用域」场景。
func (r *Repo) Mkdir(rel string) {
	full := filepath.Join(r.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(full, 0o755); err != nil {
		r.t.Fatal(err)
	}
}

// Read 读仓库内文件。
func (r *Repo) Read(rel string) string {
	b, err := os.ReadFile(filepath.Join(r.Dir, filepath.FromSlash(rel)))
	if err != nil {
		r.t.Fatalf("读 %s 失败: %v", rel, err)
	}
	return string(b)
}

// GitOut 直接跑一条 git 命令，供用例断言或诊断。失败即红，不返回空串冒充「干净」。
func (r *Repo) GitOut(args ...string) string {
	c := r.cmd("git", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v 失败: %v\n%s", args, err, out)
	}
	return string(out)
}

// Exists 判断仓库内路径是否存在。
func (r *Repo) Exists(rel string) bool {
	_, err := os.Stat(filepath.Join(r.Dir, filepath.FromSlash(rel)))
	return err == nil
}

// Run 以该仓库为工作目录执行 ritsu。
func (r *Repo) Run(args ...string) Result {
	r.t.Helper()
	return Run(r.t, r.Dir, r.Home, args...)
}

// RunText 用于人类可读输出（如 --help），不做 JSON 校验。
func (r *Repo) RunText(args ...string) Result {
	r.t.Helper()
	return runText(r.t, r.Dir, r.Home, args...)
}

func (r *Repo) cmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Dir = r.Dir
	return c
}

func (r *Repo) run(name string, args ...string) {
	c := r.cmd(name, args...)
	if out, err := c.CombinedOutput(); err != nil {
		r.t.Fatalf("%s %v 失败: %v\n%s", name, args, err, out)
	}
}

// Snapshot 记录仓库内所有文件的内容哈希，用于「零写盘」断言。
func (r *Repo) Snapshot() map[string]string {
	r.t.Helper()
	snap := map[string]string{}
	filepath.Walk(r.Dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return nil
		}
		rel, _ := filepath.Rel(r.Dir, p)
		b, rerr := os.ReadFile(p)
		if rerr == nil {
			snap[filepath.ToSlash(rel)] = hashBytes(b)
		}
		return nil
	})
	return snap
}
