// Package ratchet 是存量违规记账。存量放行、新增零容忍、删条目需显式确认。
package ratchet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"ritsu/internal/contract"
	"ritsu/internal/fsx"
)

// Path 是 baseline 的落盘位置。
const Path = ".ritsu/ratchet-baseline.json"

// Entry 是一条被放行的存量违规。reason 必填。
type Entry struct {
	Rule   string `json:"rule"`
	File   string `json:"file"`
	Ctx    string `json:"ctx"`
	Reason string `json:"reason"`
}

// Baseline 是账本。
type Baseline struct {
	Entries []Entry `json:"entries"`
}

// Fingerprint 用「规则 + 文件 + 消息」算指纹，故意不含行号：
// 文件顶部插一行注释不应让整本账失效。
func Fingerprint(d contract.Diagnostic) string {
	h := sha256.Sum256([]byte(d.RuleID + "|" + d.File + "|" + normalize(d.Message)))
	return hex.EncodeToString(h[:8])
}

func normalize(s string) string {
	b := []byte(s)
	out := b[:0]
	prevSpace := false
	for _, c := range b {
		if c == ' ' || c == '\t' {
			if prevSpace {
				continue
			}
			prevSpace = true
			out = append(out, ' ')
			continue
		}
		prevSpace = false
		out = append(out, c)
	}
	return string(out)
}

// Load 读取账本。不存在返回空账本。
func Load(files *fsx.FS, root string) (*Baseline, error) {
	b := &Baseline{}
	if !files.Exists(root, Path) {
		return b, nil
	}
	data, err := files.Read(root, Path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, b); err != nil {
		return nil, contract.NewErr(contract.CodeUsage, "baseline 解析失败: %v", err)
	}
	return b, nil
}

// Save 写账本。
func Save(files *fsx.FS, root string, b *Baseline) error {
	sort.Slice(b.Entries, func(i, j int) bool {
		if b.Entries[i].File != b.Entries[j].File {
			return b.Entries[i].File < b.Entries[j].File
		}
		return b.Entries[i].Rule < b.Entries[j].Rule
	})
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(root, Path, append(data, '\n'))
}

// metaRules 是「检测机制自身失效」类诊断。它们描述的是扫描器坏了，
// 不是代码存量债，永远不能写进棘轮账本。
var metaRules = map[string]bool{
	"baseline-shrunk":       true,
	"rule-selfproof":        true,
	"graph-selfproof":       true,
	"rule-test-orphan":      true,
	"rule-test-malformed":   true,
	"rule-test-out-of-scope": true,
	"tier1-tampered":        true,
}

// Split 把诊断拆成「可计债的代码违规」与「不可计债的元诊断」。
func Split(diags []contract.Diagnostic) (billable, meta []contract.Diagnostic) {
	for _, d := range diags {
		if metaRules[d.RuleID] {
			meta = append(meta, d)
			continue
		}
		billable = append(billable, d)
	}
	return billable, meta
}

// Diff 返回新增违规与被删除的账目。
func Diff(current []contract.Diagnostic, b *Baseline) ([]contract.Diagnostic, []Entry) {
	known := map[string]bool{}
	for _, e := range b.Entries {
		known[e.Rule+"|"+e.File+"|"+e.Ctx] = true
	}
	var fresh []contract.Diagnostic
	stillPresent := map[string]bool{}
	for _, d := range current {
		// 一枚指纹同时用作「新增判定」与「仍然存在判定」的键
		k := d.RuleID + "|" + d.File + "|" + Fingerprint(d)
		if known[k] {
			stillPresent[k] = true
			continue
		}
		fresh = append(fresh, d)
	}
	var shrunk []Entry
	for _, e := range b.Entries {
		if !stillPresent[e.Rule+"|"+e.File+"|"+e.Ctx] {
			shrunk = append(shrunk, e)
		}
	}
	return fresh, shrunk
}

// Prune 返回删掉 shrunk 后的账本副本。
func Prune(b *Baseline, shrunk []Entry) *Baseline {
	drop := map[string]bool{}
	for _, e := range shrunk {
		drop[e.Rule+"|"+e.File+"|"+e.Ctx] = true
	}
	out := &Baseline{}
	for _, e := range b.Entries {
		if drop[e.Rule+"|"+e.File+"|"+e.Ctx] {
			continue
		}
		out.Entries = append(out.Entries, e)
	}
	return out
}

// Build 从当前违规生成账本。每条必须带理由。
func Build(current []contract.Diagnostic, reason string) *Baseline {
	if reason == "" {
		reason = "存量技术债：纳管时已存在，新增零容忍"
	}
	b := &Baseline{}
	seen := map[string]bool{}
	for _, d := range current {
		fp := Fingerprint(d)
		k := d.RuleID + "|" + d.File + "|" + fp
		if seen[k] {
			continue
		}
		seen[k] = true
		b.Entries = append(b.Entries, Entry{
			Rule:   d.RuleID,
			File:   d.File,
			Ctx:    fp,
			Reason: reason,
		})
	}
	return b
}
