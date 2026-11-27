// Package diag 是双通道诊断：CI 归档用 SARIF 2.1.0，代理用紧凑 JSON。
// 紧凑通道必须保留 flow_steps，否则代理只能看到孤立行号。
package diag

import (
	"strconv"

	"ritsu/internal/contract"
)

// ToSARIF 构造 SARIF 2.1.0 报告。
func ToSARIF(diags []contract.Diagnostic) map[string]any {
	rules := map[string]bool{}
	results := make([]any, 0, len(diags))
	for _, d := range diags {
		rules[d.RuleID] = true
		level := "error"
		if d.Severity == "warning" {
			level = "warning"
		}
		line := d.Line
		if line < 1 {
			line = 1 // SARIF 要求 startLine >= 1
		}
		results = append(results, map[string]any{
			"ruleId":  d.RuleID,
			"level":   level,
			"message": map[string]any{"text": d.Message},
			"locations": []any{
				map[string]any{
					"physicalLocation": map[string]any{
						"artifactLocation": map[string]any{"uri": d.File},
						"region":           map[string]any{"startLine": line},
					},
				},
			},
		})
	}
	ruleList := make([]any, 0, len(rules))
	for id := range rules {
		ruleList = append(ruleList, map[string]any{"id": id})
	}
	return map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs": []any{
			map[string]any{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":           "ritsu",
						"informationUri": "https://example.invalid/ritsu",
						"rules":          ruleList,
					},
				},
				"results": results,
			},
		},
	}
}

// Validate 按 SARIF 2.1.0 的必填结构做机械校验。
// 不引入 schema 库：零依赖单二进制是硬约束。
func Validate(sarif map[string]any) []string {
	var problems []string
	if sarif["version"] != "2.1.0" {
		problems = append(problems, "version 必须是 2.1.0")
	}
	if _, ok := sarif["$schema"].(string); !ok {
		problems = append(problems, "缺少 $schema")
	}
	runs, ok := sarif["runs"].([]any)
	if !ok || len(runs) == 0 {
		return append(problems, "runs 必须是非空数组")
	}
	for _, r := range runs {
		run, ok := r.(map[string]any)
		if !ok {
			problems = append(problems, "runs 元素必须是对象")
			continue
		}
		tool, _ := run["tool"].(map[string]any)
		driver, _ := tool["driver"].(map[string]any)
		if name, _ := driver["name"].(string); name == "" {
			problems = append(problems, "tool.driver.name 必填")
		}
		results, _ := run["results"].([]any)
		for i, res := range results {
			item, ok := res.(map[string]any)
			if !ok {
				problems = append(problems, "results 元素必须是对象")
				continue
			}
			if id, _ := item["ruleId"].(string); id == "" {
				problems = append(problems, "results["+strconv.Itoa(i)+"].ruleId 必填")
			}
			lvl, _ := item["level"].(string)
			if lvl != "error" && lvl != "warning" && lvl != "note" {
				problems = append(problems, "results["+strconv.Itoa(i)+"].level 非法")
			}
			msg, _ := item["message"].(map[string]any)
			if text, _ := msg["text"].(string); text == "" {
				problems = append(problems, "results["+strconv.Itoa(i)+"].message.text 必填")
			}
			locs, _ := item["locations"].([]any)
			if len(locs) == 0 {
				problems = append(problems, "results["+strconv.Itoa(i)+"].locations 不能为空")
				continue
			}
			loc, _ := locs[0].(map[string]any)
			phys, _ := loc["physicalLocation"].(map[string]any)
			art, _ := phys["artifactLocation"].(map[string]any)
			if uri, _ := art["uri"].(string); uri == "" {
				problems = append(problems, "results["+strconv.Itoa(i)+"].artifactLocation.uri 必填")
			}
			region, _ := phys["region"].(map[string]any)
			if n, ok := region["startLine"].(int); !ok || n < 1 {
				problems = append(problems, "results["+strconv.Itoa(i)+"].region.startLine 必须 >= 1")
			}
		}
	}
	return problems
}

// Compact 生成代理通道的紧凑结构：剥离冗余元字段，保留成因链。
func Compact(diags []contract.Diagnostic) []any {
	out := make([]any, 0, len(diags))
	for _, d := range diags {
		item := map[string]any{
			"engine":   d.Engine,
			"rule_id":  d.RuleID,
			"severity": d.Severity,
			"file":     d.File,
			"line":     d.Line,
			"message":  d.Message,
		}
		if len(d.FlowSteps) > 0 {
			item["flow_steps"] = d.FlowSteps
		}
		if d.FixHint != "" {
			item["fix_hint"] = d.FixHint
		}
		out = append(out, item)
	}
	return out
}
