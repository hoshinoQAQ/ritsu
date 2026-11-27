package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ritsu/internal/config"
	"ritsu/internal/contract"
	"ritsu/internal/engine"
	"ritsu/internal/fsx"
	"ritsu/internal/lint"
	"ritsu/internal/lint/diag"
	"ritsu/internal/ownership"
	"ritsu/internal/ratchet"
)

// MaxDiagnostics 是代理通道的截断上限。
const MaxDiagnostics = 30

// CheckOptions 是 check 参数。
type CheckOptions struct {
	Scope      []string
	Rule       string
	Format     string
	Strict     bool
	AcceptDebt bool
}

// Check 运行静态检查并返回双通道结果。
func Check(d Deps, o CheckOptions) (*contract.Envelope, error) {
	env := contract.OK("check")

	arch, err := config.LoadArchitecture(d.Root)
	if err != nil {
		if e := contract.As(err); e.Code != contract.CodeNotFound {
			return nil, err
		}
		arch = nil
	}
	// 配置快照已进内存：此后磁盘上的 architecture.yaml 再变也不影响本次判定
	debugSleep()

	l := lint.New(arch, d.Module)
	l.Only = o.Rule
	l.Strict = o.Strict

	diags, scanned, err := l.Run(d.Root, o.Scope)
	if err != nil {
		e := contract.As(err)
		if e.Code == contract.CodeEmptyScope {
			env.Status = "failed"
			env.Error = e
			env.Diagnostics = []contract.Diagnostic{}
			env.WithNext("ritsu init --stack ts --dry-run", "ritsu check --scope .")
			return env, nil
		}
		return nil, err
	}

	// 外部引擎：失败/超时/畸形输出都是运行故障，不是"没发现问题"
	if engines, lerr := engine.Load(d.FS, d.Root); lerr != nil {
		return nil, lerr
	} else if len(engines.Engines) > 0 {
		got, rerr := engines.Run(d.Exec, d.Root)
		if rerr != nil {
			return nil, rerr
		}
		diags = append(diags, got...)
		diags = lint.Dedupe(diags)
		// 引擎诊断同样受 --strict 约束，否则告警能绕过闸门
		diags = lint.ApplySeverityPolicy(diags, o.Strict)
	}

	// 棘轮：只看新增，账目减少需显式确认
	baseline, berr := ratchet.Load(d.FS, d.Root)
	if berr != nil {
		// 账本损坏必须报错：当成空账本会让全部存量违规变成「新增」或「已放行」
		return nil, berr
	}
	if len(baseline.Entries) > 0 || o.AcceptDebt {
		all := diags
		fresh, shrunk := ratchet.Diff(all, baseline)
		if o.AcceptDebt {
			// 显式确认：以当前真实违规重建账本。元诊断（扫描器失效/篡改）绝不入账，
			// 否则一次 --accept-debt 就能把自证机制永久关掉。
			billable, meta := ratchet.Split(all)
			if err := ratchet.Save(d.FS, d.Root, ratchet.Build(billable, "")); err != nil {
				return nil, err
			}
			diags = meta
		} else {
			diags = fresh
			if len(shrunk) > 0 {
				diags = append(diags, contract.Diagnostic{
					Engine:   "ratchet",
					RuleID:   "baseline-shrunk",
					Severity: "error",
					File:     ratchet.Path,
					Line:     1,
					Message:  "账本有 " + strconv.Itoa(len(shrunk)) + " 条记录已不存在，需显式确认缩表",
					FixHint:  "确认后用 --accept-debt 重跑",
				})
			}
		}
	}

	// 规则与样本的双向自证：孤儿样本、失效规则都在这里暴露
	if st, serr := lint.SelfTest(d.FS, d.Root); serr != nil {
		return nil, serr
	} else if len(st) > 0 {
		diags = append(diags, st...)
	}

	// Tier1 只读层：签名不符即视为篡改
	if ok, bad, verr := ownership.Verify(d.FS, d.Root); verr == nil && !ok {
		diags = append(diags, contract.Diagnostic{
			Engine:   "ownership",
			RuleID:   "tier1-tampered",
			Severity: "error",
			File:     bad,
			Line:     1,
			Message:  "Tier1 只读层文件内容与签名不符",
			FixHint:  "恢复原内容；确需变更请人工刷新 .ritsu/signatures.json",
		})
	}

	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].File != diags[j].File {
			return diags[i].File < diags[j].File
		}
		return diags[i].Line < diags[j].Line
	})

	// 截断：代理通道默认只回传前 30 条，全量落盘给指针
	total := len(diags)
	if len(diags) > MaxDiagnostics {
		full := diags
		path, werr := writeFullLog(d, full)
		if werr != nil {
			return nil, werr
		}
		diags = diags[:MaxDiagnostics]
		env.Truncated = true
		env.FullLogPath = path
	}

	env.Diagnostics = diags
	if env.Diagnostics == nil {
		env.Diagnostics = []contract.Diagnostic{}
	}
	// violations 用真实总数：截断后报 30 会把 500 条违规掩盖成 30 条
	env.Data = map[string]any{"scanned": scanned, "violations": total}

	switch o.Format {
	case "sarif":
		sarif := diag.ToSARIF(diags)
		if problems := diag.Validate(sarif); len(problems) > 0 {
			return nil, contract.NewErr(contract.CodeInternal,
				"SARIF 2.1.0 自校验失败: %s", strings.Join(problems, "; ")).
				WithHint("这是 ritsu 的缺陷，请修 diag.ToSARIF")
		}
		env.Data["sarif"] = sarif
	case "agent":
		env.Data["compact"] = diag.Compact(diags)
	}

	if hasError(diags) {
		env.Status = "failed"
		env.Error = contract.NewErr(contract.CodeViolations,
			"发现 %d 条违规（已扫描 %d 个文件）", total, scanned)
		env.WithNext(
			"按 fix_hint 修复后重跑: ritsu check --format agent",
			"只看单条规则: ritsu check --rule <id>",
		)
	}
	return env, nil
}

// debugSleep 读取故障注入延迟，仅供 E2E 构造「运行中配置被改」的时间窗口。
func debugSleep() {
	v := config.Env("RITSU_DEBUG_SLOW_MS", "")
	if v == "" {
		return
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return
	}
	time.Sleep(time.Duration(n) * time.Millisecond)
}

func hasError(diags []contract.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func writeFullLog(d Deps, diags []contract.Diagnostic) (string, error) {
	body, err := json.MarshalIndent(diags, "", "  ")
	if err != nil {
		return "", err
	}
	rel := ".ritsu/logs/check-full.json"
	if err := d.FS.Write(d.Root, rel, append(body, '\n')); err != nil {
		return "", err
	}
	return filepath.Join(d.Root, filepath.FromSlash(rel)), nil
}

// VerifyOptions 是 verify 参数。
type VerifyOptions struct {
	Strict bool
}

// Verify 校验红绿证据链并产出审阅证据包。
// 全程不修改用户工作树：开始与结束各取一次快照，不一致即视为评估越权。
func Verify(d Deps, o VerifyOptions) (*contract.Envelope, error) {
	env := contract.OK("verify")
	const evidencePath = ".ritsu/redgreen.json"

	if !d.FS.Exists(d.Root, evidencePath) {
		env.Status = "failed"
		env.Error = contract.NewErr(contract.CodeRedGreen,
			"缺少红绿证据链 .ritsu/redgreen.json").
			WithHint("先跑一次失败测试并留存红灯日志，再跑实现")
		env.WithNext("ritsu plan --goal \"<目标>\"")
		return env, nil
	}

	before := worktreeDigest(d)
	// 故障注入窗口：验证"验收期间工作树被改动"能被检出
	debugSleep()

	var ev redGreen
	evidence, rerr := d.FS.Read(d.Root, evidencePath)
	if rerr != nil {
		return nil, rerr
	}
	if err := json.Unmarshal(evidence, &ev); err != nil {
		return nil, contract.NewErr(contract.CodeUsage, "证据链解析失败: %v", err)
	}

	fail := func(code contract.Code, msg string, hint string) (*contract.Envelope, error) {
		keepScene(d, msg)
		env.Status = "failed"
		env.Error = contract.NewErr(code, "%s", msg).WithHint(hint)
		env.WithNext("ritsu verify --strict")
		return env, nil
	}

	if strings.TrimSpace(ev.RedCommand) == "" || strings.TrimSpace(ev.GreenCommand) == "" {
		return fail(contract.CodeRedGreen, "证据链缺少命令字段", "补充 red_command 与 green_command")
	}
	// 验收命令本身不得是破坏性操作
	for _, c := range []string{ev.RedCommand, ev.GreenCommand} {
		for _, bad := range []string{"reset --hard", "push --force", "clean -f", "push -f"} {
			if strings.Contains(c, bad) {
				return fail(contract.CodeRedGreen,
					"验收命令包含破坏性操作: "+bad,
					"验收只读运行，不得改动或丢弃工作树")
			}
		}
	}
	if ev.RedCommand != ev.GreenCommand {
		return fail(contract.CodeRedGreen,
			"红灯与绿灯命令不一致: "+ev.RedCommand+" vs "+ev.GreenCommand,
			"两份证据必须来自同一条测试命令")
	}
	redLog, rerr := d.FS.Read(d.Root, ev.RedLog)
	if rerr != nil || len(strings.TrimSpace(string(redLog))) == 0 {
		return fail(contract.CodeRedGreen, "红灯日志缺失或为空: "+ev.RedLog, "重跑失败用例并保存完整日志")
	}
	greenLog, gerr := d.FS.Read(d.Root, ev.GreenLog)
	if gerr != nil || len(strings.TrimSpace(string(greenLog))) == 0 {
		return fail(contract.CodeRedGreen, "绿灯日志缺失或为空: "+ev.GreenLog, "重跑通过用例并保存完整日志")
	}
	if ev.RedTime.After(ev.GreenTime) {
		return fail(contract.CodeRedGreen, "红灯时间晚于绿灯时间", "必须先证明失败，再证明通过")
	}

	// 真实服务必须能被拉起并通过健康检查
	for _, s := range ev.Services {
		if err := probeService(s); err != nil {
			return fail(contract.CodeRedGreen,
				"服务 "+s.Name+" 不可用: "+err.Error(),
				"确认服务已启动且健康检查地址可达")
		}
	}

	// --strict：任何遗留告警都不放行
	if o.Strict && len(ev.Warnings) > 0 {
		return fail(contract.CodeRedGreen,
			"--strict 下仍有 "+strconv.Itoa(len(ev.Warnings))+" 条遗留告警",
			"逐条消除告警或去掉 --strict")
	}

	// 评估器只读：工作树必须与开始时逐字节一致
	if after := worktreeDigest(d); after != before {
		return fail(contract.CodeRedGreen,
			"验收期间工作树被改动，评估器越权",
			"评估器只能读；改动请由实现方在验收前提交")
	}

	bundle := map[string]any{
		"intent":   scrub(ev.Intent),
		"scope":    scrub(orDefault(ev.Scope, ".")),
		"plan":     scrub(ev.Plan),
		"changes":  scrubAll(ev.Changes),
		"checks":   scrubAll([]string{ev.RedCommand, ev.GreenCommand}),
		"evidence": map[string]any{"red_log": ev.RedLog, "green_log": ev.GreenLog},
		"services": serviceReport(ev.Services),
		"side_effects": scrubAll(ev.SideEffects),
		"warnings":     scrubAll(ev.Warnings),
	}
	body, _ := json.MarshalIndent(bundle, "", "  ")
	if err := d.FS.Write(d.Root, ".ritsu/artifact.json", append(body, '\n')); err != nil {
		return nil, err
	}
	// 成功：清理临时工作区，不留垃圾
	_ = d.FS.RemoveAll(d.Root, verifyWorkDir)

	env.Data = map[string]any{"artifact": ".ritsu/artifact.json", "strict": o.Strict}
	env.WithNext("ritsu check --format agent")
	return env, nil
}

// verifyWorkDir 是验收期间的临时工作区。成功清理，失败保留现场。
const verifyWorkDir = ".ritsu/verify-workdir"

// keepScene 失败时把现场留在固定位置并给出可读路径。
func keepScene(d Deps, reason string) {
	body := "验收失败现场\n\n原因: " + reason + "\n"
	_ = d.FS.Write(d.Root, verifyWorkDir+"/failure.txt", []byte(body))
}

// probeService 真实发起一次 HTTP 请求做健康检查，不用 stub。
func probeService(s Service) error {
	timeout := time.Duration(s.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(s.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return contract.NewErr(contract.CodeInternal, "健康检查返回 %d", resp.StatusCode)
	}
	return nil
}

// worktreeDigest 计算工作树指纹，用于证明验收全程只读。
func worktreeDigest(d Deps) string {
	files, err := d.FS.List(d.Root, ".")
	if err != nil {
		return "list-error"
	}
	h := sha256.New()
	for _, rel := range files {
		if strings.HasPrefix(rel, verifyWorkDir) ||
			strings.HasPrefix(rel, ".ritsu/logs/") ||
			rel == ".ritsu/artifact.json" ||
			strings.HasSuffix(rel, fsx.TempSuffix) {
			continue
		}
		data, rerr := d.FS.Read(d.Root, rel)
		if rerr != nil {
			continue
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write([]byte(d.FS.Sign(data)))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// secretPatterns 命中即脱敏。artifact 会进评审，不能带凭据。
var secretPatterns = []string{"token", "api_key", "apikey", "secret", "password", "authorization", "bearer", "sk-"}

// scrub 把疑似凭据替换为占位符。
func scrub(s string) string {
	lower := strings.ToLower(s)
	for _, p := range secretPatterns {
		if !strings.Contains(lower, p) {
			continue
		}
		idx := strings.Index(lower, p)
		tail := s[idx+len(p):]
		lead := s[:idx+len(p)]
		// 凭据值通常紧跟等号/冒号/空格
		cut := strings.IndexAny(tail, "=: \t")
		if cut < 0 {
			return lead + "[REDACTED]"
		}
		return lead + tail[:cut+1] + "[REDACTED]"
	}
	return s
}

func scrubAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, scrub(v))
	}
	return out
}

func serviceReport(list []Service) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]any{"name": s.Name, "url": s.URL, "ok": true})
	}
	return out
}

type Service struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	TimeoutMS int    `json:"timeout_ms"`
}

type redGreen struct {
	Intent       string    `json:"intent"`
	Scope        string    `json:"scope"`
	Plan         string    `json:"plan"`
	Changes      []string  `json:"changes"`
	RedCommand   string    `json:"red_command"`
	RedLog       string    `json:"red_log"`
	RedTime      timeVal   `json:"red_time"`
	GreenCommand string    `json:"green_command"`
	GreenLog     string    `json:"green_log"`
	GreenTime    timeVal   `json:"green_time"`
	SideEffects  []string  `json:"side_effects"`
	Warnings     []string  `json:"warnings"`
	Services     []Service `json:"services"`
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// Commands 返回全部子命令，供 --help 与 schema 共用一份事实源。
func Commands() []string {
	return []string{
		"init", "adopt", "sync", "check", "verify",
		"plan", "handoff", "schema", "version",
	}
}
