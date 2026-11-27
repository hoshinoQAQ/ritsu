package contract

import "time"

// Executor 是创建进程的唯一端口。实现位于 execx。
type Executor interface {
	Run(dir string, argv []string) (Result, error)
	// RunTimeout 带独立超时执行；超时必须以明确错误返回，不得无限等待。
	RunTimeout(dir string, argv []string, timeout time.Duration) (Result, error)
}

// Result 是进程原始产物，不含任何语义解析。
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// FileSystem 是写盘与路径校验的唯一端口。实现位于 fsx。
type FileSystem interface {
	Write(root, rel string, data []byte) error
	Read(root, rel string) ([]byte, error)
	Exists(root, rel string) bool
	List(root, rel string) ([]string, error)
	MkdirAll(root, rel string) error
	Remove(root, rel string) error
	Sign(data []byte) string
	Within(root, rel string) (string, error)
}

// Clock 是时间与随机的唯一来源。实现位于 clockx。
type Clock interface {
	Now() time.Time
}

// VCS 是 git 操作的唯一端口。实现位于 vcs。
type VCS interface {
	IsClean(root string) (bool, error)
	Init(root string) error
	Add(root string, rels ...string) error
	// AddRetry 在索引被占用时重试，仍失败则明确报错而不静默。
	AddRetry(root string, rels ...string) error
	Commit(root, msg string) error
	Head(root string) (string, error)
}

// Linter 是静态检查端口。实现位于 lint。
type Linter interface {
	Run(root string, scope []string) ([]Diagnostic, error)
}

// Evaluator 是黑盒评估端口。实现位于 verify/evaluator。
type Evaluator interface {
	Evaluate(redGreen RedGreenEvidence) (bool, string)
}

// RedGreenEvidence 是红绿证据链。两份日志缺一不可。
type RedGreenEvidence struct {
	RedCommand  string
	RedLog      string
	RedTime     time.Time
	GreenCommand string
	GreenLog    string
	GreenTime   time.Time
}

// Reporter 是输出端口。CLI 与 MCP 各自实现。
type Reporter interface {
	Emit(env *Envelope) int
}
