// Package logx 只写 stderr，且强制脱敏。不暴露 io.Writer。
package logx

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var out io.Writer = os.Stderr
var verbose bool

// SetVerbose 打开详细日志。非 verbose 时只输出 warn 及以上。
func SetVerbose(v bool) { verbose = v }

// SetWriter 仅供测试替换。
func SetWriter(w io.Writer) { out = w }

func log(level, format string, args ...any) {
	msg := redact(fmt.Sprintf(format, args...))
	fmt.Fprintf(out, "level=%s msg=%q\n", level, msg)
}

// Info 在 verbose 下输出。
func Info(format string, args ...any) {
	if verbose {
		log("info", format, args...)
	}
}

// Warn 始终输出。
func Warn(format string, args ...any) { log("warn", format, args...) }

// Error 始终输出。
func Error(format string, args ...any) { log("error", format, args...) }

// redact 抹掉疑似密钥片段，防止日志泄漏。
func redact(s string) string {
	for _, key := range []string{"token=", "password=", "api_key=", "secret="} {
		idx := 0
		for {
			i := strings.Index(strings.ToLower(s[idx:]), key)
			if i < 0 {
				break
			}
			start := idx + i + len(key)
			end := start
			for end < len(s) && s[end] != ' ' && s[end] != '"' && s[end] != '\n' {
				end++
			}
			s = s[:start] + "***" + s[end:]
			idx = start + 3
			if idx >= len(s) {
				break
			}
		}
	}
	return s
}
