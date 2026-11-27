package fsx

import (
	"os"
	"path/filepath"
)

// SkipDirs 是任何代码遍历都跳过的目录名：版本库元数据与依赖安装目录。
// 收敛在此处，避免各包各写一遍导致规则漂移（例如某处忘了跳 node_modules）。
var SkipDirs = []string{".git", "node_modules"}

// WalkFiles 深度遍历 root 下的普通文件，跳过 SkipDirs 与 extraSkip 中的目录名。
// 回调收到绝对路径；遍历期的读取错误一律跳过而非中断（并发删除、权限不足很常见）。
func WalkFiles(root string, extraSkip []string, fn func(path string, info os.FileInfo) error) error {
	skip := make(map[string]bool, len(SkipDirs)+len(extraSkip))
	for _, d := range SkipDirs {
		skip[d] = true
	}
	for _, d := range extraSkip {
		skip[d] = true
	}
	return filepath.Walk(root, func(p string, info os.FileInfo, werr error) error {
		if werr != nil {
			return nil
		}
		if info.IsDir() {
			if skip[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		return fn(p, info)
	})
}
