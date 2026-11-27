// Package ownership 判定三级文件所有权并维护内容签名。
package ownership

import (
	"encoding/json"
	"strings"

	"ritsu/internal/contract"
	"ritsu/internal/fsx"
)

// Tier 是所有权层级。
type Tier int

const (
	Tier1 Tier = 1 // 只读治理层，代理禁改
	Tier2 Tier = 2 // 三向合并配置层
	Tier3 Tier = 3 // 自由演进区
)

// SignaturesPath 是签名清单位置。
const SignaturesPath = ".ritsu/signatures.json"

// Classify 按路径前缀判定层级。顺序敏感：Tier1 优先。
func Classify(rel string) Tier {
	switch {
	case strings.HasPrefix(rel, ".ritsu/hooks/"),
		strings.HasPrefix(rel, ".ritsu/rules/"),
		strings.HasPrefix(rel, ".ritsu/rule-tests/"),
		rel == SignaturesPath,
		strings.HasPrefix(rel, ".github/workflows/ritsu-gate"),
		rel == "CODEOWNERS":
		return Tier1
	case rel == "AGENTS.md",
		rel == "package.json",
		rel == "pnpm-workspace.yaml",
		rel == "tsconfig.json",
		rel == "turbo.json",
		rel == "moon.yml",
		rel == "nx.json",
		rel == "stylelint.config.js",
		rel == ".ritsu/architecture.yaml",
		rel == ".ritsu/answers.yml",
		rel == "pyproject.toml":
		return Tier2
	default:
		return Tier3
	}
}

// IsReadOnly 判断该路径是否禁止代理改写。
func IsReadOnly(rel string) bool { return Classify(rel) == Tier1 }

// Signatures 是内容签名清单。
type Signatures struct {
	Files map[string]string `json:"files"`
}

// Load 读取签名清单。
func Load(files *fsx.FS, root string) (*Signatures, error) {
	s := &Signatures{Files: map[string]string{}}
	if !files.Exists(root, SignaturesPath) {
		return s, nil
	}
	data, err := files.Read(root, SignaturesPath)
	if err != nil {
		return nil, err
	}
	// 解析失败必须报错：空 map 会让 Verify 判定「全部合规」，等于关掉篡改检测
	if err := json.Unmarshal(data, s); err != nil {
		return nil, contract.NewErr(contract.CodeInternal,
			"签名清单解析失败: %v", err).WithPath(SignaturesPath)
	}
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	return s, nil
}

// Save 写签名清单。
func Save(files *fsx.FS, root string, s *Signatures) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(root, SignaturesPath, append(data, '\n'))
}

// Refresh 为已存在文件重算签名。
func Refresh(files *fsx.FS, root string, paths []string) (*Signatures, error) {
	s, err := Load(files, root)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		data, rerr := files.Read(root, p)
		if rerr != nil {
			// 读不到就不能静默跳过：该文件会永远没有签名，之后篡改也放行
			return nil, contract.NewErr(contract.CodeInternal,
				"签名失败，无法读取 %s: %v", p, rerr).WithPath(p)
		}
		s.Files[p] = files.Sign(data)
	}
	if err := Save(files, root, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Verify 校验 Tier1 文件未被篡改，返回首个违规路径。
func Verify(files *fsx.FS, root string) (bool, string, error) {
	s, err := Load(files, root)
	if err != nil {
		return false, "", err
	}
	// 清单为空即视为检测失效：Tier1 文件存在却没有签名，是配置被删而非合规
	if len(s.Files) == 0 && files.Exists(root, ".ritsu/hooks/pre-tool-use.sh") {
		return false, SignaturesPath, nil
	}
	for p, want := range s.Files {
		data, rerr := files.Read(root, p)
		if rerr != nil {
			return false, p, nil
		}
		if files.Sign(data) != want {
			return false, p, nil
		}
	}
	return true, "", nil
}
