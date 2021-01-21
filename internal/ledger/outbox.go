// 账本变更同事务写入出箱，再异步投递。
// 作者: Span
// 日期: 2021.01.06
package ledger

import (
	"time"
)

// OutboxEvent 同事务出箱。出入金仍以 journal 为准，bus 只投递。

type OutboxEvent struct {
	ID string `json:"id"`
	AggregateType string `json:"aggregate_type"`
	AggregateID string `json:"aggregate_id"`
	Kind   string `json:"kind"`
	Action string `json:"action"`
	UserID string `json:"user_id"`
	TraceID string         `json:"trace_id,omitempty"`
	State   string         `json:"state"`
	Payload map[string]any `json:"payload,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}
