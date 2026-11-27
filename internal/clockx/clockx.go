// Package clockx 是时间与随机的唯一来源。
package clockx

import "time"

// Clock 实现 contract.Clock。fake 非空时固定时间，保证 E2E 可重现。
type Clock struct{ fixed time.Time }

// New 用 fake 构造（RFC3339 或空）。
func New(fake string) *Clock {
	c := &Clock{}
	if fake != "" {
		if t, err := time.Parse(time.RFC3339, fake); err == nil {
			c.fixed = t
		}
	}
	return c
}

// Now 返回当前时间。
func (c *Clock) Now() time.Time {
	if !c.fixed.IsZero() {
		return c.fixed
	}
	return time.Now()
}
