package acctpol

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// AuditAction 审计动作类型。

type AuditAction string

const (
	// AuditPublish 发布新代次。
	AuditPublish AuditAction = "publish"
	// AuditRollback 回滚到历史代次。
	AuditRollback AuditAction = "rollback"
	// AuditPinUser 用户 Pin 到指定代次。
	AuditPinUser AuditAction = "pin_user"
	// AuditUnpinUser 解除用户 Pin。
	AuditUnpinUser AuditAction = "unpin_user"
	// AuditGrayRatio 调整灰度比例。
	AuditGrayRatio AuditAction = "gray_ratio"
)

// AuditEntry 一条不可变审计记录。

type AuditEntry struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
	Action AuditAction `json:"action"`
	Actor       string `json:"actor"`
	FromVersion int    `json:"from_version,omitempty"`
	ToVersion   int    `json:"to_version,omitempty"`
	Subject     string `json:"subject,omitempty"` // 用户 Pin 时为 UserID
	Reason string `json:"reason,omitempty"`
}

// AuditStore 政策审计仓储（内存 + SQL）。

type AuditStore struct {
	mu   sync.Mutex
	rows []AuditEntry
	db   *sql.DB
	clock func() time.Time
}

// NewAuditStore 创建审计仓储。

  func NewAuditStore(db *sql.DB) *AuditStore {
	   return &AuditStore{
		   db:    db,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// Append 追加一条审计记录。actor 不能为空，也不能是 "system"。

   func (a *AuditStore) Append(ctx context.Context, e AuditEntry) (AuditEntry, error) {
	  e.Actor = strings.TrimSpace(e.Actor)
	  if e.Actor == "" || strings.EqualFold(e.Actor, "system") {
		  return AuditEntry{}, fmt.Errorf("acctpol audit: actor required and must not be system")
	  }
	   if e.ID == "" {
		  e.ID = uuid.NewString()
	   }
	  if e.At.IsZero() {
		   e.At = a.clock()
	   }
	  a.mu.Lock()
	  a.rows = append(a.rows, e)
	  a.mu.Unlock()
	  if a.db != nil {
		  if err := a.persistEntry(ctx, e); err != nil {
			  return e, fmt.Errorf("acctpol audit persist: %w", err)
		   }
	  }
	  return e, nil
  }

// List 按 action 过滤（空字符串=全部），最多返回 limit 条（降序）。
   func (a *AuditStore) List(action AuditAction, limit int) []AuditEntry {
	  if limit <= 0 {
		  limit = 100
	   }
	   a.mu.Lock()
	   cp := append([]AuditEntry(nil), a.rows...)
	   a.mu.Unlock()
	   var out []AuditEntry
	  for i := len(cp) - 1; i >= 0 && len(out) < limit; i-- {
		   e := cp[i]
		  if action == "" || e.Action == action {
			  out = append(out, e)
		  }
	   }
	  return out
  }

// RecordPublish 记录代次发布事件。
  func (a *AuditStore) RecordPublish(ctx context.Context, actor string, toVersion int, reason string) error {
	  _, err := a.Append(ctx, AuditEntry{
		  Action: AuditPublish,
		  Actor: actor,
		  ToVersion: toVersion,
		  Reason:    reason,
	  })
	   if err == nil {
		   metrics.AcctPolVersionPublished.Inc()
	  }
	  return err
  }

// RecordRollback 记录回滚事件。
  func (a *AuditStore) RecordRollback(ctx context.Context, actor string, fromVersion, toVersion int, reason string) error {
	  _, err := a.Append(ctx, AuditEntry{
		  Action:      AuditRollback,
		   Actor:       actor,
		   FromVersion: fromVersion,
		   ToVersion: toVersion,
		  Reason:    reason,
	  })
	   if err == nil {
		   metrics.AcctPolRollback.Inc()
	  }
	  return err
  }

// RecordPinUser 记录用户 Pin 变更。

   func (a *AuditStore) RecordPinUser(ctx context.Context, actor, userID string, version int) error {
	  _, err := a.Append(ctx, AuditEntry{
		   Action: AuditPinUser,
		  Actor:  actor,
		   ToVersion: version,
		   Subject:   userID,
	  })
	   return err
   }

// RecordUnpinUser 记录解除 Pin。

   func (a *AuditStore) RecordUnpinUser(ctx context.Context, actor, userID string, fromVersion int) error {
	   _, err := a.Append(ctx, AuditEntry{
		   Action: AuditUnpinUser,
		   Actor: actor,
		   FromVersion: fromVersion,
		  Subject: userID,
	   })
	  return err
  }

// Load 从数据库加载审计历史。

  func (a *AuditStore) Load(ctx context.Context) error {
	   if a.db == nil {
		  return nil
	  }
	  rows, err := a.db.QueryContext(ctx,
		   `SELECT id, at, action, actor, from_version, to_version, subject, reason
		 FROM acctpol_audit ORDER BY at ASC`)
	   if err != nil {
		   return fmt.Errorf("acctpol audit load: %w", err)
	   }
	   defer rows.Close()
	   a.mu.Lock()
	  defer a.mu.Unlock()
	  for rows.Next() {
		  var e AuditEntry
		  var fromVer, toVer sql.NullInt64
		  var subject, reason sql.NullString
		   if err := rows.Scan(&e.ID, &e.At, &e.Action, &e.Actor, &fromVer, &toVer, &subject, &reason); err != nil {
			  return err
		   }
		  if fromVer.Valid {
			  e.FromVersion = int(fromVer.Int64)
		  }
		  if toVer.Valid {
			   e.ToVersion = int(toVer.Int64)
		   }
		  if subject.Valid {
			  e.Subject = subject.String
		  }
		  if reason.Valid {
			   e.Reason = reason.String
		   }
		   a.rows = append(a.rows, e)
	  }
	   return rows.Err()
  }

   func (a *AuditStore) persistEntry(ctx context.Context, e AuditEntry) error {
	   _, err := a.db.ExecContext(ctx,
		   `INSERT INTO acctpol_audit
		 (id, at, action, actor, from_version, to_version, subject, reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		   e.ID, e.At, string(e.Action), e.Actor,
		  nullInt(e.FromVersion),
		  nullInt(e.ToVersion),
		   nullStr(e.Subject), nullStr(e.Reason))
	  return err
  }

  func nullInt(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

   func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
