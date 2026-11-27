// Package fsx 是文件系统原语：原子写、内容签名、路径越界拦截。
// 全仓唯一允许写盘的地方。
package fsx

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ritsu/internal/contract"
)

// FS 实现 contract.FileSystem。
type FS struct{ slowMS int }

// New 返回文件系统端口实现。slowMS>0 时在落盘前注入延迟，仅供故障注入测试。
func New(slowMS ...int) *FS {
	f := &FS{}
	if len(slowMS) > 0 {
		f.slowMS = slowMS[0]
	}
	return f
}

// Within 把相对路径解析成绝对路径，并强制其落在 root 之内。
// 这是 --scope 越界拦截的唯一实现点：字面越界与符号链接逃逸都在此拦截。
func (f *FS) Within(root, rel string) (string, error) {
	if rel == "" {
		rel = "."
	}
	if filepath.IsAbs(rel) {
		return "", contract.NewErr(contract.CodeScopeOutside, "路径必须是相对路径: %s", rel)
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if full != base && !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", contract.NewErr(contract.CodeScopeOutside, "路径越出作用域: %s", rel)
	}
	// 符号链接逃逸：字面路径没越界，但真实指向可能在仓库外
	realBase, berr := filepath.EvalSymlinks(base)
	if berr != nil {
		return full, nil // 根目录尚未物化，无可逃逸
	}
	realFull := resolveReal(full)
	if realFull != realBase && !strings.HasPrefix(realFull, realBase+string(filepath.Separator)) {
		return "", contract.NewErr(contract.CodeScopeOutside,
			"路径经符号链接逃出作用域: %s", rel)
	}
	return full, nil
}

// resolveReal 对最长已存在前缀取真实路径，再拼回尚不存在的尾部。
func resolveReal(full string) string {
	rest := ""
	cur := full
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			if rest == "" {
				return real
			}
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return full
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// TempSuffix 是原子写临时文件的后缀。进程被强杀时可能残留，启动时统一清理。
const TempSuffix = ".ritsu-tmp"

// Write 原子写入：先写同目录临时文件，再改名覆盖。
// 中途被打断时仓库里不会留下半截文件，也不会留下目录外的临时文件。
func (f *FS) Write(root, rel string, data []byte) error {
	full, err := f.Within(root, rel)
	if err != nil {
		return err
	}
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return contract.NewErr(contract.CodeInternal,
			"无法创建目录 %s: %v", rel, err).WithPath(rel)
	}
	// 目标已存在且只读：明确报错，不要依赖各平台 rename 的差异行为
	if info, statErr := os.Stat(full); statErr == nil && info.Mode().Perm()&0o200 == 0 {
		return contract.NewErr(contract.CodeInternal,
			"目标文件只读，无法写入: %s", rel).WithPath(rel).
			WithHint("先解除只读属性或改用可写路径")
	}
	// 临时文件名必须唯一：并发进程共用同一后缀会互相锁死
	tmp := full + "." + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36) + TempSuffix
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return contract.NewErr(contract.CodeInternal,
			"写入 %s 失败: %v", rel, err).WithPath(rel)
	}
	if f.slowMS > 0 {
		// 故障注入：留出可被强杀的时间窗口，验证「要么旧内容要么新内容」
		time.Sleep(time.Duration(f.slowMS) * time.Millisecond)
	}
	// 不做 Remove：rename 本身就是原子替换（Windows 走 MoveFileEx 的 REPLACE_EXISTING），
	// 先删再改名会留出「文件不存在」窗口，破坏「要么旧内容要么新内容」的承诺
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return contract.NewErr(contract.CodeInternal,
			"落盘 %s 失败: %v", rel, err).WithPath(rel)
	}
	return nil
}

// CleanupTemps 清理上次运行被强杀后残留的临时文件。启动时调用。
func (f *FS) CleanupTemps(root string) int {
	base, err := filepath.Abs(root)
	if err != nil {
		return 0
	}
	n := 0
	_ = WalkFiles(base, nil, func(p string, info os.FileInfo) error {
		if !strings.HasSuffix(p, TempSuffix) {
			return nil
		}
		// 3 秒内的临时文件可能正被并发进程写入，不能动
		if time.Since(info.ModTime()) < 3*time.Second {
			return nil
		}
		if os.Remove(p) == nil {
			n++
		}
		return nil
	})
	return n
}

// Read 读取文件。
func (f *FS) Read(root, rel string) ([]byte, error) {
	full, err := f.Within(root, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

// Exists 判断路径是否存在。
func (f *FS) Exists(root, rel string) bool {
	full, err := f.Within(root, rel)
	if err != nil {
		return false
	}
	_, statErr := os.Stat(full)
	return statErr == nil
}

// List 列出目录下所有文件（相对 root 的 slash 路径），已排序。
func (f *FS) List(root, rel string) ([]string, error) {
	full, err := f.Within(root, rel)
	if err != nil {
		return nil, err
	}
	var out []string
	err = WalkFiles(full, nil, func(p string, _ os.FileInfo) error {
		r, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(r))
		return nil
	})
	sort.Strings(out)
	return out, err
}

// MkdirAll 建目录。
func (f *FS) MkdirAll(root, rel string) error {
	full, err := f.Within(root, rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, 0o755)
}

// Remove 删除文件或空目录。
func (f *FS) Remove(root, rel string) error {
	full, err := f.Within(root, rel)
	if err != nil {
		return err
	}
	return os.Remove(full)
}

// RemoveAll 递归删除仓库内路径。仅供临时工作区清理使用。
func (f *FS) RemoveAll(root, rel string) error {
	full, err := f.Within(root, rel)
	if err != nil {
		return err
	}
	return os.RemoveAll(full)
}

// Sign 计算内容签名。
func (f *FS) Sign(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
