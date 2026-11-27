package app

import (
	"strings"

	"ritsu/internal/contract"
	"ritsu/internal/plan"
)

// PlanCreate 生成执行计划并按需初始化接力卡。
func PlanCreate(d Deps, goal string, dryRun bool) (*contract.Envelope, error) {
	env := contract.OK("plan")
	if dryRun {
		env.Data = map[string]any{
			"would_create": plan.ActiveDir + "/" + plan.Slug(goal) + ".md",
			"dry_run":      true,
		}
		return env, nil
	}
	rel, err := plan.Create(d.FS, d.Root, goal)
	if err != nil {
		return nil, err
	}
	if err := plan.EnsureContinuity(d.FS, d.Root); err != nil {
		return nil, err
	}
	env.Data = map[string]any{"created": rel, "continuity": plan.ContinuityPath}
	env.WithNext("ritsu check --format agent")
	return env, nil
}

// Handoff 只更新接力卡的单个字段。
func Handoff(d Deps, field, value string) (*contract.Envelope, error) {
	env := contract.OK("handoff")
	if field == "" {
		return nil, contract.NewErr(contract.CodeUsage, "handoff 需要 --field").
			WithHint("合法字段: " + strings.Join(plan.Fields, "、"))
	}
	if err := plan.Update(d.FS, d.Root, field, value); err != nil {
		return nil, err
	}
	env.Data = map[string]any{"updated": field}
	return env, nil
}

// PlanComplete 把计划移入 completed。
func PlanComplete(d Deps, name string) (*contract.Envelope, error) {
	env := contract.OK("plan")
	if name == "" {
		return nil, contract.NewErr(contract.CodeUsage, "需要 --complete <文件名>")
	}
	to, err := plan.Complete(d.FS, d.Root, name)
	if err != nil {
		return nil, err
	}
	env.Data = map[string]any{"moved_to": to}
	return env, nil
}

// PlanReset 判断是否需要上下文重置。
func PlanReset(d Deps) (*contract.Envelope, error) {
	env := contract.OK("plan")
	need, reason := plan.ResetCheck(d.FS, d.Root)
	env.Data = map[string]any{"reset": need, "reason": reason}
	return env, nil
}

// Fields 暴露接力卡字段，供 schema 使用。顺序本身是语义（固定八段），不得排序。
func Fields() []string {
	out := make([]string, len(plan.Fields))
	copy(out, plan.Fields)
	return out
}
