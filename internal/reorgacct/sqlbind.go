package reorgacct

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// EventQuery 复合事件查询
// ────────────────────────────────────────────────────────────────────────────
// EventQuery 事件多条件查询参数。

type EventQuery struct {
	Chain string
	State string
	SourceKind string
	// CreatedAfter 若非零，只返回 created_at > CreatedAfter 的事件。
	CreatedAfter time.Time
	// CreatedBefore 若非零，只返回 created_at < CreatedBefore 的事件。
	CreatedBefore time.Time
	Limit         int
	Offset int
}

// QueryEvents 按多条件查询事件列表（供 API 和 Worker 精细过滤）。

   func QueryEvents(ctx context.Context, db *sql.DB, q EventQuery) ([]*ReorgEvent, error) {
	   if db == nil {
		   return nil, ErrStoreUnavailable
	  }
	  if q.Limit <= 0 || q.Limit > 500 {
		   q.Limit = 50
	   }
	   conds := []string{"1=1"}
	   args := []any{}
	  if q.Chain != "" {
		   conds = append(conds, "chain = ?")
		  args = append(args, strings.ToUpper(strings.TrimSpace(q.Chain)))
	   }
	  if q.State != "" {
		   conds = append(conds, "state = ?")
		   args = append(args, q.State)
	  }
	  if q.SourceKind != "" {
		  conds = append(conds, "source_kind = ?")
		   args = append(args, q.SourceKind)
	   }
	   if !q.CreatedAfter.IsZero() {
		   conds = append(conds, "created_at > ?")
		   args = append(args,
			   q.CreatedAfter.UTC())
	  }
	  if !q.CreatedBefore.IsZero() {
		   conds = append(conds, "created_at < ?")
		  args = append(args, q.CreatedBefore.UTC())
	   }
	  args = append(args,
		  q.Limit, q.Offset)
	  query := fmt.Sprintf(
		  `SELECT id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
		        affected_outputs, affected_tracks, evidence, state, source_kind, note,
		        created_at, updated_at
		 FROM reorg_events WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		   strings.Join(conds, " AND "),
	   )
	   rows, err := db.QueryContext(ctx, query, args...)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []*ReorgEvent
	  for rows.Next() {
		  ev, err := scanEventRow(rows)
		  if err != nil {
			   return nil, err
		  }
		  out = append(out, ev)
	   }
	   return out, rows.Err()
  }

// ────────────────────────────────────────────────────────────────────────────
// ImpactRowQuery 复合影响面查询
// ────────────────────────────────────────────────────────────────────────────
// ImpactRowQuery 影响面多条件查询参数。

type ImpactRowQuery struct {
	EventID string
	UserID  string
	Asset   string
	Kind    string
	Unbound bool // 只返回 reverse_batch_id 为空的行（尚未规划的影响面）
	Limit   int
	Offset  int
}

// QueryImpactRows 按多条件查询影响面行。

  func QueryImpactRows(ctx context.Context, db *sql.DB, q ImpactRowQuery) ([]ImpactRow, error) {
	  if db == nil {
		   return nil, ErrStoreUnavailable
	  }
	  if q.Limit <= 0 || q.Limit > 500 {
		   q.Limit = 50
	   }
	  conds := []string{"1=1"}
	   args := []any{}
	   if q.EventID != "" {
		  conds = append(conds, "event_id = ?")
		  args = append(args, q.EventID)
	  }
	  if q.UserID != "" {
		   conds = append(conds, "user_id = ?")
		  args = append(args, q.UserID)
	  }
	   if q.Asset != "" {
		  conds = append(conds, "asset = ?")
		   args = append(args, strings.ToUpper(strings.TrimSpace(q.Asset)))
	   }
	  if q.Kind != "" {
		  conds = append(conds, "kind = ?")
		  args = append(args, q.Kind)
	   }
	   if q.Unbound {
		  conds = append(conds, "(reverse_batch_id IS NULL OR reverse_batch_id = '')")
	  }
	   args = append(args, q.Limit, q.Offset)
	  query := fmt.Sprintf(
		   `SELECT id, event_id, user_id, asset, kind, reference_id, affected_amount,
		        COALESCE(reverse_batch_id,''), note, created_at
		 FROM impact_rows WHERE %s ORDER BY created_at ASC LIMIT ? OFFSET ?`,
		  strings.Join(conds, " AND "),
	  )
	   rows,
		   err := db.QueryContext(ctx, query, args...)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []ImpactRow
	   for rows.Next() {
		  var r ImpactRow
		   if err := rows.Scan(&r.ID, &r.EventID, &r.UserID, &r.Asset, &r.Kind,
			   &r.ReferenceID, &r.AffectedAmount, &r.ReverseBatchID, &r.Note, &r.CreatedAt); err != nil {
			  return nil, err
		   }
		  out = append(out, r)
	  }
	  return out, rows.Err()
   }

// ────────────────────────────────────────────────────────────────────────────
// BulkUpdateEventState 批量更新事件状态（供 Worker 批量标记 error）
// ────────────────────────────────────────────────────────────────────────────
// BulkUpdateEventState 将 ids 列表中的事件批量推进到 newState。
// 使用单 UPDATE ... WHERE id IN (?) 减少往返次数。

  func BulkUpdateEventState(ctx context.Context, db *sql.DB, ids []string, newState string) error {
	  if db == nil {
		  return ErrStoreUnavailable
	   }
	   if len(ids) == 0 {
		   return nil
	  }
	  placeholders := make([]string, len(ids))
	   args := make([]any, len(ids)+2)
	   for i, id := range ids {
		  placeholders[i] = "?"
		  args[i] = id
	   }
	  args[len(ids)] = newState
	  args[len(ids)+1] = time.Now().UTC()
	   // 注意 SET 子句中 state 和 updated_at 在 WHERE 之后追加。
	  q := fmt.Sprintf(
		  `UPDATE reorg_events SET state = ?, updated_at = ? WHERE id IN (%s)`,
		   strings.Join(placeholders, ","),
	   )
	  // 重新排列 args：SET 参数在前，IN 参数在后。
	  setArgs := []any{newState, time.Now().UTC()}
	   setArgs = append(setArgs, func() []any {
		   r := make([]any, len(ids))
		  for i, id := range ids {
			  r[i] = id
		  }
		  return r
	  }()...)
	   _, err := db.ExecContext(ctx, q, setArgs...)
	   return err
   }

// ────────────────────────────────────────────────────────────────────────────
// ImpactStatsByEvent 按事件统计影响面
// ────────────────────────────────────────────────────────────────────────────
// ImpactStats 事件维度的影响面统计。
type ImpactStats struct {
	EventID      string `json:"event_id"`
	TotalRows    int    `json:"total_rows"`
	CreditedRows int    `json:"credited_rows"`
	OccupiedRows int    `json:"occupied_rows"`
	FreezeRows int `json:"freeze_rows"`
	HopPartialRows int `json:"hop_partial_rows"`
	LiveHashRows int `json:"live_hash_rows"`
	UnboundRows  int `json:"unbound_rows"` // 尚未关联批次的行数
}

// ImpactStatsByEvent 统计指定事件的影响面分布（用于监控看板）。
  func ImpactStatsByEvent(ctx context.Context, db *sql.DB, eventID string) (ImpactStats, error) {
	  stats := ImpactStats{EventID: eventID}
	   if db == nil {
		  return stats, ErrStoreUnavailable
	   }
	   q := `SELECT kind, COUNT(*),
	             SUM(CASE WHEN reverse_batch_id = '' OR reverse_batch_id IS NULL THEN 1 ELSE 0 END)
	      FROM impact_rows WHERE event_id = ? GROUP BY kind`
	   rows, err := db.QueryContext(ctx, q, eventID)
	   if err != nil {
		   return stats, err
	  }
	   defer rows.Close()
	  for rows.Next() {
		  var kind string
		  var count, unbound int
		   if err := rows.Scan(&kind, &count, &unbound); err != nil {
			  return stats, err
		  }
		  stats.TotalRows += count
		   stats.UnboundRows += unbound
		  switch kind {
		   case KindCredited:
			   stats.CreditedRows = count
		  case KindOccupied:
			   stats.OccupiedRows = count
		  case KindFreeze:
			  stats.FreezeRows = count
		   case KindHopPartial:
			   stats.HopPartialRows = count
		   case KindLiveHash:
			   stats.LiveHashRows = count
		  }
	   }
	  return stats, rows.Err()
   }
