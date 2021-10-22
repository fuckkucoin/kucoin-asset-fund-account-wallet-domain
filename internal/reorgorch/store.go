package reorgorch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Store SQL 持久化层。
type Store struct {
	db *sql.DB
}

// NewStore 构造 Store；db 不得为 nil。

  func NewStore(db *sql.DB) *Store {
	  return &Store{db: db}
  }

// ────────────────────────────────────────────────────────────────────────────
// ReverseBatch CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveBatch 写入新批次。
   func (s *Store) SaveBatch(ctx context.Context, b *ReverseBatch) error {
	  if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	   }
	  rowIDsJSON, _ := json.Marshal(b.ImpactRowIDs)
	  q := `INSERT INTO reverse_batches
		(id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs, idem_key, note, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		  b.ID, b.EventID, b.State, rowIDsJSON,
		   b.TotalLegs,
		  b.DoneLegs, b.FailedLegs,
		  b.IdemKey, b.Note, b.CreatedAt, b.UpdatedAt,
	  )
	  return err
   }

// GetBatch 按 ID 查询批次。
   func (s *Store) GetBatch(ctx context.Context, id string) (*ReverseBatch, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	   q := `SELECT id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
	             idem_key, COALESCE(note,''), created_at, updated_at
	      FROM reverse_batches WHERE id = ? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, id)
	  return scanBatch(row)
  }

// FindBatchByEventID 幂等查询：按 event_id 查找批次（无则返回 nil）。

   func (s *Store) FindBatchByEventID(ctx context.Context, eventID string) (*ReverseBatch, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	   q := `SELECT id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
	             idem_key, COALESCE(note,''), created_at, updated_at
	      FROM reverse_batches WHERE event_id = ? ORDER BY created_at ASC LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, eventID)
	   b, err := scanBatch(row)
	  if err == sql.ErrNoRows {
		  return nil, nil
	   }
	   return b, err
  }

// UpdateBatchState 更新批次状态与 updated_at。
   func (s *Store) UpdateBatchState(ctx context.Context, id, state string) error {
	  if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	   }
	  q := `UPDATE reverse_batches SET state = ?, updated_at = ? WHERE id = ?`
	   _, err := s.db.ExecContext(ctx, q, state, time.Now().UTC(), id)
	   return err
  }

// UpdateBatchLegCount 更新批次总腿数（Plan 时回写）。

  func (s *Store) UpdateBatchLegCount(ctx context.Context, id string, total int) error {
	  if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	   }
	   q := `UPDATE reverse_batches SET total_legs = ?, updated_at = ? WHERE id = ?`
	  _, err := s.db.ExecContext(ctx, q, total, time.Now().UTC(), id)
	  return err
   }

// IncrBatchLegCounters 原子递增批次的 done_legs / failed_legs 计数器。

  func (s *Store) IncrBatchLegCounters(ctx context.Context, batchID string, doneDelta, failedDelta int) error {
	  if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	  }
	   q := `UPDATE reverse_batches SET done_legs = done_legs + ?, failed_legs = failed_legs + ?, updated_at = ? WHERE id = ?`
	   _, err := s.db.ExecContext(ctx, q, doneDelta, failedDelta, time.Now().UTC(), batchID)
	  return err
  }

// ListBatches 分页查询批次列表（按 created_at DESC）。

   func (s *Store) ListBatches(ctx context.Context, eventID, state string, limit, offset int) ([]*ReverseBatch, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  args := []any{}
	  conds := []string{"1=1"}
	  if eventID != "" {
		   conds = append(conds, "event_id = ?")
		   args = append(args, eventID)
	   }
	  if state != "" {
		  conds = append(conds, "state = ?")
		  args = append(args, state)
	   }
	  args = append(args, limit, offset)
	  q := fmt.Sprintf(
		   `SELECT id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
		        idem_key, COALESCE(note,''), created_at, updated_at
		 FROM reverse_batches WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		  strings.Join(conds, " AND "),
	  )
	   rows,
		   err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []*ReverseBatch
	  for rows.Next() {
		   b, err := scanBatchRow(rows)
		  if err != nil {
			   return nil, err
		  }
		   out = append(out, b)
	  }
	   return out, rows.Err()
   }

// ListExecutingBatches 查询 executing 状态批次（供 Worker 轮询）。
   func (s *Store) ListExecutingBatches(ctx context.Context, limit int) ([]*ReverseBatch, error) {
	   if limit <= 0 {
		  limit = 10
	   }
	  return s.ListBatches(ctx, "", BatchStateExecuting, limit, 0)
   }

// ────────────────────────────────────────────────────────────────────────────
// ReverseLeg CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveLegs 批量写入腿（单事务）。
   func (s *Store) SaveLegs(ctx context.Context, legs []ReverseLeg) error {
	  if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	  }
	  if len(legs) == 0 {
		  return nil
	  }
	   tx,
		  err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		  return err
	   }
	   defer func() {
		  if err != nil {
			  _ = tx.Rollback()
		   }
	   }()
	  q := `INSERT INTO reverse_legs
		(id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
		 reference_id, amount, user_id, asset, high_risk, result_note, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   stmt, err := tx.PrepareContext(ctx, q)
	  if err != nil {
		   return err
	  }
	   defer stmt.Close()
	  for _, leg := range legs {
		  highRisk := 0
		   if leg.HighRisk {
			   highRisk = 1
		  }
		   if _, err = stmt.ExecContext(ctx,
			   leg.ID, leg.BatchID, leg.ImpactRowID, leg.Kind, leg.ExecutionOrder,
			  leg.State,
			   leg.IdemKey, leg.ReferenceID, leg.Amount,
			  leg.UserID, leg.Asset, highRisk, leg.ResultNote, leg.CreatedAt,
		  ); err != nil {
			  return err
		   }
	  }
	  return tx.Commit()
  }

// GetLeg 按 ID 查询单条腿。

  func (s *Store) GetLeg(ctx context.Context, id string) (*ReverseLeg, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  q := `SELECT id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
	             reference_id, amount, user_id, asset, high_risk,
	             COALESCE(result_note,''), created_at,
	             started_at, finished_at
	      FROM reverse_legs WHERE id = ? LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, id)
	   return scanLegRow(row)
   }

// ListLegs 查询批次所有腿（按 execution_order ASC）。
  func (s *Store) ListLegs(ctx context.Context, batchID string) ([]ReverseLeg, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	   q := `SELECT id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
	             reference_id, amount, user_id, asset, high_risk,
	             COALESCE(result_note,''), created_at,
	             started_at, finished_at
	      FROM reverse_legs WHERE batch_id = ? ORDER BY execution_order ASC`
	   rows, err := s.db.QueryContext(ctx, q, batchID)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []ReverseLeg
	   for rows.Next() {
		   leg, err := scanLegRow(rows)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, *leg)
	  }
	   return out, rows.Err()
   }

// UpdateLegState 更新腿状态与结果说明。

  func (s *Store) UpdateLegState(ctx context.Context, id, state, resultNote string) error {
	  if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	   }
	  q := `UPDATE reverse_legs SET state = ?, result_note = ? WHERE id = ?`
	  _, err := s.db.ExecContext(ctx, q, state, resultNote, id)
	  return err
  }

// MarkLegStarted 记录腿开始执行时间。
   func (s *Store) MarkLegStarted(ctx context.Context, legID string, t time.Time) error {
	   if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	  }
	   q := `UPDATE reverse_legs SET started_at = ? WHERE id = ?`
	  _, err := s.db.ExecContext(ctx, q, t, legID)
	  return err
   }

// MarkLegFinished 记录腿结束时间。

  func (s *Store) MarkLegFinished(ctx context.Context, legID string, t time.Time) error {
	   if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	   }
	   q := `UPDATE reverse_legs SET finished_at = ? WHERE id = ?`
	  _, err := s.db.ExecContext(ctx, q, t, legID)
	   return err
  }

// ────────────────────────────────────────────────────────────────────────────
// ReverseGuard CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveGuard 写入护栏记录（唯一约束冲突由调用方解释为 ErrGuardConflict）。

   func (s *Store) SaveGuard(ctx context.Context, g *ReverseGuard) error {
	   if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	   }
	  q := `INSERT INTO reverse_guards (id, impact_row_id, leg_kind, batch_id, leg_id, locked_at)
	      VALUES (?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q, g.ID, g.ImpactRowID, g.LegKind, g.BatchID, g.LegID, g.LockedAt)
	   return err
   }

// GetGuard 按 (impact_row_id, leg_kind) 查护栏；不存在返回 nil。

   func (s *Store) GetGuard(ctx context.Context, impactRowID, legKind string) (*ReverseGuard, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	  q := `SELECT id, impact_row_id, leg_kind, batch_id, leg_id, locked_at
	      FROM reverse_guards WHERE impact_row_id = ? AND leg_kind = ? LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, impactRowID, legKind)
	   var g ReverseGuard
	  err := row.Scan(&g.ID, &g.ImpactRowID, &g.LegKind, &g.BatchID, &g.LegID, &g.LockedAt)
	  if err == sql.ErrNoRows {
		   return nil, nil
	  }
	   if err != nil {
		   return nil, err
	  }
	   return &g, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// IncidentTicket CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveIncident 写入事故单。
   func (s *Store) SaveIncident(ctx context.Context, t *IncidentTicket) error {
	  if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	   }
	  q := `INSERT INTO incident_tickets (id, batch_id, leg_id, kind, description, state, created_at, updated_at)
	      VALUES (?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		  t.ID, t.BatchID, t.LegID, t.Kind, t.Description, t.State, t.CreatedAt, t.UpdatedAt)
	  return err
  }

// ListIncidents 查询批次事故单。
  func (s *Store) ListIncidents(ctx context.Context, batchID string) ([]IncidentTicket, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	   q := `SELECT id, batch_id, COALESCE(leg_id,''), kind, description, state, created_at, updated_at 
	      FROM incident_tickets WHERE batch_id = ? ORDER BY created_at ASC`
	  rows, err := s.db.QueryContext(ctx, q, batchID)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []IncidentTicket
	   for rows.Next() {
		  var t IncidentTicket
		  if err := rows.Scan(&t.ID, &t.BatchID, &t.LegID, &t.Kind, &t.Description, &t.State, &t.CreatedAt, &t.UpdatedAt); err != nil {
			  return nil, err
		   }
		   out = append(out, t)
	   }
	  return out, rows.Err()
   }

// ────────────────────────────────────────────────────────────────────────────
// 孤儿行审计
// ────────────────────────────────────────────────────────────────────────────
// ListOrphanImpactRows 查询已关闭批次下、未被任何护栏覆盖的 ImpactRow ID（孤儿行）。
// 此查询仅作告警；不自动触发冲正。

   func (s *Store) ListOrphanImpactRows(ctx context.Context, limit int) ([]string, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	   // 查找：存在于 impact_rows 且 reverse_batch_id 指向已关闭批次，
	   // 但在 reverse_guards 中找不到任何护栏记录的行。
	  q := `SELECT ir.id
	      FROM impact_rows ir
	      JOIN reverse_batches rb ON ir.reverse_batch_id = rb.id
	      LEFT JOIN reverse_guards rg ON rg.impact_row_id = ir.id
	      WHERE rb.state IN ('committed','aborted')
	        AND rg.id IS NULL
	        AND ir.kind != 'live_hash'
	      LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   var out []string
	   for rows.Next() {
		  var id string
		   if err := rows.Scan(&id); err != nil {
			   return nil, err
		  }
		  out = append(out, id)
	  }
	  return out, rows.Err()
  }

// RecordOrphanAudit 写入孤儿行审计记录（INSERT IGNORE 防重）。

  func (s *Store) RecordOrphanAudit(ctx context.Context, impactRowID string) error {
	   if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	  }
	  q := `INSERT IGNORE INTO guard_orphan_audit (impact_row_id, detected_at) VALUES (?, ?)`
	  _, err := s.db.ExecContext(ctx, q, impactRowID, time.Now().UTC())
	  return err
  }

// ────────────────────────────────────────────────────────────────────────────
// 内部 scan 辅助
// ────────────────────────────────────────────────────────────────────────────

type batchScanner interface {
	Scan(dest ...any) error
}

   func scanBatch(row *sql.Row) (*ReverseBatch, error) {
	  return scanBatchFrom(row)
  }

   func scanBatchRow(row *sql.Rows) (*ReverseBatch, error) {
	  return scanBatchFrom(row)
  }

  func scanBatchFrom(sc batchScanner) (*ReverseBatch, error) {
	   var b ReverseBatch
	  var rowIDsJSON []byte
	  if err := sc.Scan(
		  &b.ID, &b.EventID, &b.State, &rowIDsJSON,
		  &b.TotalLegs, &b.DoneLegs, &b.FailedLegs,
		  &b.IdemKey, &b.Note, &b.CreatedAt, &b.UpdatedAt,
	   ); err != nil {
		  return nil, err
	  }
	  _ = json.Unmarshal(rowIDsJSON, &b.ImpactRowIDs)
	  return &b, nil
   }

type legScannerI interface {
	Scan(dest ...any) error
}

  func scanLegRow(sc legScannerI) (*ReverseLeg, error) {
	   var leg ReverseLeg
	  var highRisk int
	   var startedAt, finishedAt *time.Time
	   if err := sc.Scan(
		  &leg.ID, &leg.BatchID, &leg.ImpactRowID, &leg.Kind,
		  &leg.ExecutionOrder,
		  &leg.State, &leg.IdemKey,
		   &leg.ReferenceID, &leg.Amount, &leg.UserID, &leg.Asset,
		  &highRisk, &leg.ResultNote, &leg.CreatedAt,
		  &startedAt, &finishedAt,
	   ); err != nil {
		   return nil, err
	   }
	  leg.HighRisk = highRisk == 1
	   leg.StartedAt = startedAt
	  leg.FinishedAt = finishedAt
	  return &leg, nil
  }
