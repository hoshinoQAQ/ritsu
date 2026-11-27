package fault_test

import (
	"strings"
	"testing"

	"ritsu/tests/e2e/fixture"
)

// FAULT-004 引擎超时必须退出 1，不得无限等待。
func TestFAULT004EngineTimeout(t *testing.T) {
	r := managed(t)
	r.Write("tools/slow.mjs", "setTimeout(() => console.log('[]'), 5000);\n")
	r.Write(".ritsu/engines.json", `{"engines":[{"id":"slow","command":"node","args":["tools/slow.mjs"],"timeout_ms":200}]}`)

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	if code := fixture.Str(t, res.Envelope, "error.code"); code != "ENGINE_TIMEOUT" {
		t.Fatalf("FAULT-004 error.code=%s", code)
	}
	if strings.Contains(res.Stderr, "panic") || strings.Contains(res.Stdout, "panic") {
		t.Fatalf("FAULT-004 不得 panic")
	}
}

// FAULT-007 引擎输出畸形 JSON 时退出 1，绝不 panic。
func TestFAULT007EngineMalformedOutput(t *testing.T) {
	r := managed(t)
	r.Write("tools/bad.mjs", "console.log('this is definitely not a json array');\n")
	r.Write(".ritsu/engines.json", `{"engines":[{"id":"bad","command":"node","args":["tools/bad.mjs"],"timeout_ms":5000}]}`)

	res := r.Run("check")
	fixture.AssertExit(t, res, 1)

	if code := fixture.Str(t, res.Envelope, "error.code"); code != "ENGINE_MALFORMED_OUTPUT" {
		t.Fatalf("FAULT-007 error.code=%s", code)
	}
	msg := fixture.Str(t, res.Envelope, "error.message")
	if !strings.Contains(msg, "bad") {
		t.Fatalf("FAULT-007 未指明是哪个引擎: %s", msg)
	}
	if strings.Contains(res.Stderr, "goroutine") {
		t.Fatalf("FAULT-007 出现崩溃栈: %s", res.Stderr)
	}
}
