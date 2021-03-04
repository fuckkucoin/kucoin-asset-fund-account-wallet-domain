package balproofgate

import (
	"sync"
	"time"
)

// AuditKind 审计类别。
type AuditKind string

const (
	AuditIssued AuditKind = "issued"
	AuditIdempotent  AuditKind = "idempotent"
	AuditRecheckDeny AuditKind = "recheck_deny"
	AuditConsumed    AuditKind = "consumed"
	AuditRevoked     AuditKind = "revoked"
	AuditExpired AuditKind = "expired"
	AuditDriftOpen AuditKind = "drift_open"
)

// AuditEvent 一条资金面可检索记录。
type AuditEvent struct {
	Kind         AuditKind `json:"kind"`
	ProofLeaseID string    `json:"proof_lease_id"`
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Asset  string `json:"asset"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

// AuditLog 有界环形缓冲。

type AuditLog struct {
	mu sync.Mutex
	max  int
	rows []AuditEvent
}

// NewAuditLog 默认保留 2048 条。
   func NewAuditLog(max int) *AuditLog {
	  if max <= 0 {
		  max = 2048
	  }
	  return &AuditLog{max: max}
  }

// Append 追加。
  func (a *AuditLog) Append(ev AuditEvent) {
	   if a == nil {
		   return
	   }
	  if ev.At.IsZero() {
		  ev.At = time.Now().UTC()
	  }
	  a.mu.Lock()
	   defer a.mu.Unlock()
	  if len(a.rows) >= a.max {
		   a.rows = a.rows[1:]
	  }
	  a.rows = append(a.rows, ev)
   }

// Recent 倒序返回。
  func (a *AuditLog) Recent(n int) []AuditEvent {
	  if a == nil {
		   return nil
	   }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  if n <= 0 || n > len(a.rows) {
		  n = len(a.rows)
	  }
	  out := make([]AuditEvent, n)
	  for i := 0; i < n; i++ {
		  out[i] = a.rows[len(a.rows)-1-i]
	  }
	   return out
  }
