package app

import (
	"os"

	"ritsu/internal/config"
	"ritsu/internal/contract"
	"ritsu/internal/ownership"
	"ritsu/internal/template"
)
// SyncOptions 是 sync 参数。
type SyncOptions struct {
	DryRun bool
	SkipAnswered bool
}

// Sync 分层增量同步：Tier1 覆盖、Tier2 三向合并、Tier3 永不触碰。
func Sync(d Deps, o SyncOptions) (*contract.Envelope, error) {
	env := contract.OK("sync")

	answers, err := config.LoadAnswers(d.Root)
	if err != nil {
		return nil, err
	}
	opts := template.Options{Stack: answers.Stack, TeamSize: answers.TeamSize}
	rendered := template.Render(opts)

	var overwritten, merged, conflicted, kept, untouched, backedUp []string
	for _, f := range rendered {
		if isLocallyGenerated(f.Path) {
			// 由本仓真实依赖图/签名现场生成，模板内容不具权威性
			untouched = append(untouched, f.Path)
			continue
		}
		tier := ownership.Classify(f.Path)
		switch tier {
		case ownership.Tier3:
			untouched = append(untouched, f.Path)
			continue
		case ownership.Tier1:
			want := f.Content + readOnlyMarker
			cur, has, rerr := readIfExists(d, f.Path)
			if rerr != nil {
				return nil, rerr
			}
			if has && string(cur) == want {
				continue
			}
			overwritten = append(overwritten, f.Path)
			if o.DryRun {
				continue
			}
			// 覆盖前留档：Tier1 被上游改写后仍能追回本地版本
			if has {
				if err := d.FS.Write(d.Root, backupPrefix+f.Path, cur); err != nil {
					return nil, err
				}
				backedUp = append(backedUp, f.Path)
			}
			if err := d.FS.Write(d.Root, f.Path, []byte(want)); err != nil {
				return nil, err
			}
		case ownership.Tier2:
			baseBytes, hasBase, berr := readIfExists(d, ".ritsu/base/"+f.Path)
			if berr != nil {
				return nil, berr
			}
			localBytes, hasLocal, lerr := readIfExists(d, f.Path)
			if lerr != nil {
				return nil, lerr
			}
			if !hasLocal {
				merged = append(merged, f.Path)
				if !o.DryRun {
					if err := d.FS.Write(d.Root, f.Path, []byte(f.Content)); err != nil {
						return nil, err
					}
				}
				continue
			}
			base := string(baseBytes)
			local := string(localBytes)
			switch {
			case local == f.Content:
				// 一致，无需动作
			case hasBase && local == base:
				// 本地未定制，取上游
				merged = append(merged, f.Path)
				if !o.DryRun {
					if err := d.FS.Write(d.Root, f.Path, []byte(f.Content)); err != nil {
						return nil, err
					}
				}
			case hasBase && f.Content == base:
				// 上游无变化，保留本地定制
			default:
				if o.SkipAnswered {
					// 已答：本地定制就是答案，保留本地，不写冲突标记也不报错
					kept = append(kept, f.Path)
					continue
				}
				// 双方都改过：写冲突标记，退出 1
				conflicted = append(conflicted, f.Path)
				if !o.DryRun {
					body := "<<<<<<< local\n" + local + "\n=======\n" + f.Content + "\n>>>>>>> upstream\n"
					if err := d.FS.Write(d.Root, f.Path, []byte(body)); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	if !o.DryRun {
		if _, err := ownership.Refresh(d.FS, d.Root, template.Tier1Paths()); err != nil {
			return nil, err
		}
		if err := snapshotBase(d); err != nil {
			return nil, err
		}
	}

	env.Data = map[string]any{
		"overwritten": overwritten,
		"merged":      merged,
		"kept":        kept,
		"untouched":   len(untouched),
		"conflicts":   conflicted,
		"backed_up":   backedUp,
		"dry_run":     o.DryRun,
	}
	if len(conflicted) > 0 {
		env.Status = "failed"
		env.Error = contract.NewErr(contract.CodeConflict,
			"Tier 2 出现 %d 处冲突，已写入冲突标记", len(conflicted))
		env.WithNext("编辑冲突文件后重跑: ritsu sync --skip-answered")
		return env, nil
	}
	env.WithNext("ritsu check --format agent")
	return env, nil
}

// readOnlyMarker 是所有 Tier1 文件末尾的固定只读标记。
const readOnlyMarker = "\n# read-only: managed by ritsu\n"

// isLocallyGenerated 判断该文件是否由本仓现场生成、模板内容不具权威性。
func isLocallyGenerated(rel string) bool {
	return rel == ".ritsu/architecture.yaml"
}

// backupPrefix 存放 Tier1 被覆盖前的原内容，保证可追溯。
const backupPrefix = ".ritsu/backup/"

// readIfExists 读取仓库内文件。文件不存在返回 (nil, false, nil)，
// 其它读取错误原样上抛——「不存在」与「读失败」必须可区分，
// 否则磁盘故障会被当成「本地没定制」而静默覆盖用户文件。
func readIfExists(d Deps, rel string) ([]byte, bool, error) {
	data, err := d.FS.Read(d.Root, rel)
	if err == nil {
		return data, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}
