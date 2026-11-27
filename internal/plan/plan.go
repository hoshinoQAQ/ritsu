// Package plan 管理一等执行计划与跨窗口状态接力卡。
package plan

import (
	"strings"

	"ritsu/internal/contract"
	"ritsu/internal/fsx"
)

// ActiveDir 是进行中计划目录。
const ActiveDir = "docs/exec-plans/active"
// CompletedDir 是已完成计划目录。
const CompletedDir = "docs/exec-plans/completed"
// ContinuityPath 是接力卡路径。
const ContinuityPath = "agent/CONTINUITY.md"

// Fields 是接力卡的八个字段，顺序固定。
var Fields = []string{
	"当前目标", "验收标准", "首选必读文档", "已作决策",
	"当前进度", "阻塞项", "下一步行动", "仍需完成的验证",
}

// Slug 由目标文本生成稳定的文件名。
func Slug(goal string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(goal) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "plan"
	}
	if r := []rune(s); len(r) > 48 {
		// 按 rune 截断：按字节切会把多字节汉字劈成非法 UTF-8
		s = string(r[:48])
	}
	return s
}

// Create 生成执行计划文件。已存在则报错，不静默覆盖。
func Create(files *fsx.FS, root, goal string) (string, error) {
	if strings.TrimSpace(goal) == "" {
		return "", contract.NewErr(contract.CodeUsage, "--goal 不能为空")
	}
	rel := ActiveDir + "/" + Slug(goal) + ".md"
	if files.Exists(root, rel) {
		return "", contract.NewErr(contract.CodeExists, "执行计划已存在: %s", rel).
			WithHint("换一个 --goal，或先处理既有计划")
	}
	body := "# " + goal + "\n\n## 验收标准\n\n- 待补充\n\n## 进度\n\n- [ ] 待开始\n\n## 决策日志\n\n"
	if err := files.Write(root, rel, []byte(body)); err != nil {
		return "", err
	}
	return rel, nil
}

// EnsureContinuity 确保接力卡存在且恰好八个字段。
func EnsureContinuity(files *fsx.FS, root string) error {
	if files.Exists(root, ContinuityPath) {
		return nil
	}
	return files.Write(root, ContinuityPath, []byte(Render(map[string]string{})))
}

// Render 按固定顺序渲染八个字段。
func Render(values map[string]string) string {
	var b strings.Builder
	b.WriteString("# CONTINUITY\n")
	for _, f := range Fields {
		b.WriteString("\n## " + f + "\n")
		v := strings.TrimSpace(values[f])
		if v != "" {
			b.WriteString("\n" + v + "\n")
		}
	}
	return b.String()
}

// Parse 解析接力卡为字段映射。
func Parse(src string) map[string]string {
	out := map[string]string{}
	cur := ""
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			cur = strings.TrimSpace(strings.TrimPrefix(t, "## "))
			out[cur] = ""
			continue
		}
		if cur != "" && t != "" {
			if out[cur] != "" {
				out[cur] += "\n"
			}
			out[cur] += t
		}
	}
	return out
}

// Update 只更新指定字段，其余字段保持原样。
func Update(files *fsx.FS, root, field, value string) error {
	if err := EnsureContinuity(files, root); err != nil {
		return err
	}
	data, err := files.Read(root, ContinuityPath)
	if err != nil {
		return err
	}
	vals := Parse(string(data))
	if _, ok := vals[field]; !ok {
		return contract.NewErr(contract.CodeUsage, "未知字段: %s", field).
			WithHint("合法字段: " + strings.Join(Fields, "、"))
	}
	vals[field] = value
	return files.Write(root, ContinuityPath, []byte(Render(vals)))
}

// Complete 把进行中计划移入已完成目录。目标已存在则拒绝。
func Complete(files *fsx.FS, root, name string) (string, error) {
	from := ActiveDir + "/" + name
	if !files.Exists(root, from) {
		return "", contract.NewErr(contract.CodeNotFound, "计划不存在: %s", from)
	}
	to := CompletedDir + "/" + name
	if files.Exists(root, to) {
		return "", contract.NewErr(contract.CodeExists, "已完成目录已有同名计划: %s", to)
	}
	data, err := files.Read(root, from)
	if err != nil {
		return "", err
	}
	if err := files.Write(root, to, data); err != nil {
		return "", err
	}
	if err := files.Remove(root, from); err != nil {
		return "", err
	}
	return to, nil
}

// ResetCheck 判断是否需要上下文重置。
func ResetCheck(files *fsx.FS, root string) (bool, string) {
	if !files.Exists(root, ContinuityPath) {
		return false, "无接力卡，无需重置"
	}
	data, err := files.Read(root, ContinuityPath)
	if err != nil {
		return false, ""
	}
	lines := strings.Count(string(data), "\n")
	if lines > 40 {
		return true, "接力卡超过 40 行，已偏离八字段约定"
	}
	vals := Parse(string(data))
	if strings.TrimSpace(vals["阻塞项"]) != "" && strings.TrimSpace(vals["下一步行动"]) == "" {
		return true, "存在阻塞项但没有下一步行动"
	}
	return false, "状态卡健康"
}
