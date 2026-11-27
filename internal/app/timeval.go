package app

import "time"

// timeVal 让时间字段既接受 RFC3339 字符串，也接受零值。
type timeVal struct{ t time.Time }

// UnmarshalJSON 解析 RFC3339。
func (v *timeVal) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" || s == "null" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	v.t = t
	return nil
}

// MarshalJSON 输出 RFC3339。
func (v timeVal) MarshalJSON() ([]byte, error) {
	if v.t.IsZero() {
		return []byte(`""`), nil
	}
	return []byte(`"` + v.t.Format(time.RFC3339) + `"`), nil
}

// After 报告是否晚于另一时间。
func (v timeVal) After(o timeVal) bool { return v.t.After(o.t) }
