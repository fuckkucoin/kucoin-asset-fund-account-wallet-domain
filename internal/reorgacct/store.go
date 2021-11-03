package reorgacct

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
// ReorgEvent CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveEvent 写入新 ReorgEvent 及其幂等键。
  func (s *Store) SaveEvent(ctx context.Context, ev *ReorgEvent, idemKey string) error {
	   if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	  }
	   outputsJSON, err := marshalJSON(ev.AffectedOutputs)
	   if err != nil {
		  return fmt.Errorf("reorgacct store save event outputs: %w", err)
	  }
	  tracksJSON, err := marshalJSON(ev.AffectedTracks)
	  if err != nil {
		   return fmt.Errorf("reorgacct store save event tracks: %w", err)
	   }
	  evidenceJSON, err := marshalJSON(ev.Evidence)
	   if err != nil {
		   return fmt.Errorf("reorgacct store save event evidence: %w", err)
	   }
	   q := `INSERT INTO reorg_events
		(id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
		 affected_outputs, affected_tracks, evidence, state, source_kind, note,
		 idem_key, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err = s.db.ExecContext(ctx, q,
		  ev.ID, ev.Chain, ev.PivotTxHash, ev.DepthBefore, ev.DepthAfter, ev.RollbackDepth,
		   outputsJSON, tracksJSON, evidenceJSON, ev.State, ev.SourceKind, ev.Note,
		  idemKey, ev.CreatedAt, ev.UpdatedAt,
	   )
	   return err
  }

// GetEvent 按 ID 查询单个 ReorgEvent。

  func (s *Store) GetEvent(ctx context.Context, id string) (*ReorgEvent, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  q := `SELECT id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
	             affected_outputs, affected_tracks, evidence, state, source_kind, note,
	             created_at, updated_at
	      FROM reorg_events WHERE id = ? LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, id)
	  return scanEvent(row)
   }

// FindByIdemKey 幂等查询；not found 时返回 (nil, false, nil)。

   func (s *Store) FindByIdemKey(ctx context.Context, idemKey string) (*ReorgEvent, bool, error) {
	   if s == nil || s.db == nil {
		  return nil, false, ErrStoreUnavailable
	  }
	  q := `SELECT id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
	             affected_outputs, affected_tracks, evidence, state, source_kind, note,
	             created_at, updated_at
	      FROM reorg_events WHERE idem_key = ? LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, idemKey)
	   ev, err := scanEvent(row)
	   if err == sql.ErrNoRows {
		   return nil, false, nil
	   }
	  if err != nil {
		   return nil, false, err
	  }
	  return ev, true, nil
  }

// UpdateEventState 将事件状态更新为 newState，并刷新 updated_at。

  func (s *Store) UpdateEventState(ctx context.Context, id, newState string) error {
	   if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	   }
	   q := `UPDATE reorg_events SET state = ?, updated_at = ? WHERE id = ?`
	   _, err := s.db.ExecContext(ctx, q, newState, time.Now().UTC(), id)
	   return err
   }

// ListEvents 分页查询事件列表（按 created_at DESC）。

  func (s *Store) ListEvents(ctx context.Context, chain, state string, limit, offset int) ([]*ReorgEvent, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  args := []any{}
	   where := "1=1"
	  if chain != "" {
		  where += " AND chain = ?"
		   args = append(args, chain)
	   }
	  if state != "" {
		  where += " AND state = ?"
		  args = append(args, state)
	   }
	   args = append(args, limit, offset)
	  q := fmt.Sprintf(`SELECT id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
	             affected_outputs, affected_tracks, evidence, state, source_kind, note,
	             created_at, updated_at
	      FROM reorg_events WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, where)
	   rows, err := s.db.QueryContext(ctx, q, args...)
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

// ListPendingEvents 查询处于 pending 状态的事件（供 Worker 轮询）。
   func (s *Store) ListPendingEvents(ctx context.Context, limit int) ([]*ReorgEvent, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	  if limit <= 0 {
		  limit = 20
	   }
	   q := `SELECT id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
	             affected_outputs, affected_tracks, evidence, state, source_kind, note,
	             created_at, updated_at
	      FROM reorg_events WHERE state = ? ORDER BY created_at ASC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, EventStatePending, limit)
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
// ImpactRow CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveImpactRows 批量写入影响面明细行（单事务）。

  func (s *Store) SaveImpactRows(ctx context.Context, rows []ImpactRow) error {
	   if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	   }
	  if len(rows) == 0 {
		  return nil
	   }
	   tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		  return err
	  }
	   defer func() {
		  if err != nil {
			  _ = tx.Rollback()
		  }
	   }()
	  q := `INSERT INTO impact_rows 
		(id, event_id, user_id, asset, kind, reference_id, affected_amount, note, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`
	   stmt,
		  err := tx.PrepareContext(ctx, q)
	   if err != nil {
		  return err
	   }
	  defer stmt.Close()
	   for _, r := range rows {
		   if _, err = stmt.ExecContext(ctx,
			  r.ID, r.EventID, r.UserID, r.Asset, r.Kind,
			  r.ReferenceID, r.AffectedAmount, r.Note, r.CreatedAt,
		  ); err != nil {
			   return err
		   }
	   }
	  return tx.Commit()
  }

// ListImpactRows 查询某 event 的所有影响面行。
   func (s *Store) ListImpactRows(ctx context.Context, eventID string) ([]ImpactRow, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  q := `SELECT id, event_id, user_id, asset, kind, reference_id, affected_amount,
	             COALESCE(reverse_batch_id,''), note, created_at
	      FROM impact_rows WHERE event_id = ? ORDER BY created_at ASC`
	   rows, err := s.db.QueryContext(ctx, q, eventID)
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

// ListImpactRowsByReference 按 reference_id 查询影响面行（供钩子只读消费）。

   func (s *Store) ListImpactRowsByReference(ctx context.Context, referenceID string) ([]ImpactRow, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	   q := `SELECT id, event_id, user_id, asset, kind, reference_id, affected_amount,
	             COALESCE(reverse_batch_id,''), note, created_at
	      FROM impact_rows WHERE reference_id = ? ORDER BY created_at ASC`
	  rows, err := s.db.QueryContext(ctx, q, referenceID)
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

// MarkImpactRowBatch 将 ImpactRow.ReverseBatchID 回写（由 reorgorch 调用）。

   func (s *Store) MarkImpactRowBatch(ctx context.Context, rowID, batchID string) error {
	   if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	  }
	   q := `UPDATE impact_rows SET reverse_batch_id = ? WHERE id = ?`
	   _,
		   err := s.db.ExecContext(ctx, q, batchID, rowID)
	  return err
   }

// ────────────────────────────────────────────────────────────────────────────
// 内部 scan 工具
// ────────────────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

   func scanEvent(row *sql.Row) (*ReorgEvent, error) {
	   return scanEventFrom(row)
   }

  func scanEventRow(row *sql.Rows) (*ReorgEvent, error) {
	  return scanEventFrom(row)
   }

   func scanEventFrom(sc scanner) (*ReorgEvent, error) {
	  var ev ReorgEvent
	  var outputsJSON, tracksJSON, evidenceJSON []byte
	   if err := sc.Scan(
		   &ev.ID, &ev.Chain, &ev.PivotTxHash,
		   &ev.DepthBefore, &ev.DepthAfter, &ev.RollbackDepth,
		  &outputsJSON, &tracksJSON, &evidenceJSON,
		  &ev.State,
		  &ev.SourceKind, &ev.Note,
		   &ev.CreatedAt, &ev.UpdatedAt,
	   ); err != nil {
		   return nil, err
	  }
	   _ = json.Unmarshal(outputsJSON, &ev.AffectedOutputs)
	   _ = json.Unmarshal(tracksJSON, &ev.AffectedTracks)
	   _ = json.Unmarshal(evidenceJSON, &ev.Evidence)
	   return &ev, nil
   }

   func marshalJSON(v any) ([]byte, error) {
	  if v == nil {
		   return []byte("null"), nil
	  }
	   return json.Marshal(v)
  }
