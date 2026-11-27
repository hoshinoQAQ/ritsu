package contract

import "encoding/json"

// Envelope 是所有命令的 stdout 形态。字段顺序稳定，便于 golden 比对。
type Envelope struct {
	SchemaVersion string         `json:"schemaVersion"`
	Status        string         `json:"status"` // ok | failed
	Command       string         `json:"command"`
	Version       string         `json:"version,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
	Diagnostics   []Diagnostic   `json:"diagnostics"`
	Truncated     bool           `json:"truncated,omitempty"`
	FullLogPath   string         `json:"full_log_path,omitempty"`
	NextActions   []string       `json:"next_actions,omitempty"`
	Error         *Err           `json:"error,omitempty"`
	Commands      []string       `json:"commands,omitempty"`
	Fields        []string       `json:"fields,omitempty"`

	// keep 非空时按 --fields 白名单序列化，其余键一律不出现。
	keep map[string]bool
}

// MarshalJSON 在 --fields 裁剪后只输出被保留的键。
func (e Envelope) MarshalJSON() ([]byte, error) {
	if len(e.keep) == 0 {
		type alias Envelope
		return json.Marshal(alias(e))
	}
	full, err := json.Marshal(e.rawAll())
	if err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(full, &all); err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for k := range e.keep {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	return json.Marshal(out)
}

func (e Envelope) rawAll() Envelope {
	e.keep = nil
	return e
}

// OK 成功信封。
func OK(cmd string) *Envelope {
	return &Envelope{
		SchemaVersion: SchemaVersion,
		Status:        "ok",
		Command:       cmd,
		Diagnostics:   []Diagnostic{},
	}
}

// Fail 失败信封，必须带 error。
func Fail(cmd string, e *Err) *Envelope {
	return &Envelope{SchemaVersion: SchemaVersion, Status: "failed", Command: cmd, Error: e}
}

// WithNext 追加可执行建议。
func (e *Envelope) WithNext(actions ...string) *Envelope {
	e.NextActions = append(e.NextActions, actions...)
	return e
}

// Prune 实现 --fields：只保留指定顶层键，其余键必须不存在。
func (e *Envelope) Prune(fields []string) {
	if len(fields) == 0 {
		return
	}
	keep := map[string]bool{}
	for _, f := range fields {
		keep[f] = true
	}
	// schemaVersion 是协议必需项，任何裁剪下都保留
	keep["schemaVersion"] = true
	e.keep = keep
}
