// 所有写操作通过 persist.SQLDB 执行；行级围栏由 fence_epoch/fence_token 列实现。
// 表结构见 migrations/0089_aa_xfer_halfhop.sql。

// Author: CainReed
// Date: 2021.12.30

package xferclose

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/shopspring/decimal"
)

// SQLStore SQL 实现，满足 Store 接口。

type SQLStore struct {
	db *persist.SQLDB
}

// NewSQLStore 构造 SQL Store。
  func NewSQLStore(db *persist.SQLDB) *SQLStore {
	   return &SQLStore{db: db}
   }

// ── HalfHop SQL ──────────────────────────────────────────────────────────────

  func (s *SQLStore) PutHop(ctx context.Context, h HalfHop) error {
	  amtStr := h.Amount.String()
	  deadlineStr := h.SLADeadline.UTC().Format(time.RFC3339Nano)
	   detectedStr := h.DetectedAt.UTC().Format(time.RFC3339Nano)
	   updatedStr := h.UpdatedAt.UTC().Format(time.RFC3339Nano)
	  q := s.db.Conn(ctx)
	  _, err := q.ExecContext(ctx,
		  `INSERT INTO aa_half_hops
		 (id, hop_id, transfer_id, user_id, asset, amount, from_book, to_book,
		  kind, state, sla_breached, sla_deadline, sla_rule_id,
		  closeout_plan_id, escalate_reason, x_shard, detected_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE id=id`,
		  h.ID, h.HopID, h.TransferID, h.UserID, h.Asset, amtStr,
		   h.FromBook,
		   h.ToBook, string(h.Kind), string(h.State),
		   boolInt(h.SLABreached), deadlineStr, h.SLARuleID,
		   h.CloseoutPlanID, h.EscalateReason, boolInt(h.XShard),
		  detectedStr, updatedStr,
	  )
	  if err != nil && isDuplicate(err) {
		  return ErrConflict
	   }
	  return err
   }

   func (s *SQLStore) UpdateHop(ctx context.Context, h HalfHop) error {
	   updatedStr := h.UpdatedAt.UTC().Format(time.RFC3339Nano)
	  var closedStr *string
	   if h.ClosedAt != nil {
		   t := h.ClosedAt.UTC().Format(time.RFC3339Nano)
		  closedStr = &t
	  }
	   q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx,
		   `UPDATE aa_half_hops SET
		 state=?, sla_breached=?, closeout_plan_id=?, escalate_reason=?,
		 updated_at=?, closed_at=?
		 WHERE id=?`,
		  string(h.State), boolInt(h.SLABreached), h.CloseoutPlanID, h.EscalateReason,
		  updatedStr, closedStr, h.ID,
	   )
	   return err
   }

   func (s *SQLStore) GetHop(ctx context.Context, id string) (HalfHop, error) {
	  q := s.db.Conn(ctx)
	   row := q.QueryRowContext(ctx,
		  `SELECT id, hop_id, transfer_id, user_id, asset, amount, from_book, to_book,
		 kind, state, sla_breached, sla_deadline, sla_rule_id,
		 closeout_plan_id, escalate_reason, x_shard, detected_at, updated_at, closed_at
		 FROM aa_half_hops WHERE id=?`, id)
	   return scanHop(row)
   }

   func (s *SQLStore) GetByHopID(ctx context.Context, hopID string) (HalfHop, error) {
	   q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		   `SELECT id, hop_id, transfer_id, user_id, asset, amount, from_book, to_book,
		 kind, state, sla_breached, sla_deadline, sla_rule_id,
		 closeout_plan_id, escalate_reason, x_shard, detected_at, updated_at, closed_at
		 FROM aa_half_hops WHERE hop_id=?`, hopID)
	  return scanHop(row)
  }

   func (s *SQLStore) ListActive(ctx context.Context, offset, limit int) ([]HalfHop, error) {
	   if limit <= 0 || limit > 500 {
		  limit = 100
	  }
	  q := s.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		   `SELECT id, hop_id, transfer_id, user_id, asset, amount, from_book, to_book, 
		 kind, state, sla_breached, sla_deadline, sla_rule_id,
		 closeout_plan_id, escalate_reason, x_shard, detected_at, updated_at, closed_at
		 FROM aa_half_hops WHERE state NOT IN ('closed','escalated')
		 ORDER BY detected_at ASC LIMIT ? OFFSET ?`, limit, offset)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  return scanHops(rows)
   }

   func (s *SQLStore) ListByState(ctx context.Context, state HopState, offset, limit int) ([]HalfHop, error) {
	  if limit <= 0 || limit > 500 {
		  limit = 100
	   }
	   q := s.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx,
		   `SELECT id, hop_id, transfer_id, user_id, asset, amount, from_book, to_book,
		 kind, state, sla_breached, sla_deadline, sla_rule_id,
		 closeout_plan_id, escalate_reason, x_shard, detected_at, updated_at, closed_at
		 FROM aa_half_hops WHERE state=?
		 ORDER BY detected_at ASC LIMIT ? OFFSET ?`, string(state), limit, offset)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanHops(rows)
  }

// ── CloseoutPlan SQL ─────────────────────────────────────────────────────────

  func (s *SQLStore) PutPlan(ctx context.Context, p CloseoutPlan) error {
	   now := p.CreatedAt.UTC().Format(time.RFC3339Nano)
	   q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx,
		   `INSERT INTO aa_closeout_plans
		 (id, hop_id, half_hop_id, state, selected_action, reason, actor,
		  attempts, last_error, abort_reason, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		   p.ID, p.HopID, p.HalfHopID, string(p.State), string(p.SelectedAction),
		  p.Reason, p.Actor, p.Attempts, p.LastError, p.AbortReason, now, now,
	  )
	   if err != nil && isDuplicate(err) {
		  return ErrPlanRunning
	   }
	   return err
   }

  func (s *SQLStore) UpdatePlan(ctx context.Context, p CloseoutPlan) error {
	  updated := p.UpdatedAt.UTC().Format(time.RFC3339Nano)
	   var doneAt *string
	  if p.DoneAt != nil {
		   t := p.DoneAt.UTC().Format(time.RFC3339Nano)
		  doneAt = &t
	   }
	   q := s.db.Conn(ctx)
	  _, err := q.ExecContext(ctx,
		   `UPDATE aa_closeout_plans SET
		 state=?, attempts=?, last_error=?, abort_reason=?, done_at=?, updated_at=?
		 WHERE id=?`,
		   string(p.State), p.Attempts, p.LastError, p.AbortReason, doneAt, updated, p.ID,
	  )
	   return err
  }

  func (s *SQLStore) GetPlan(ctx context.Context, id string) (CloseoutPlan, error) {
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		   `SELECT id, hop_id, half_hop_id, state, selected_action, reason, actor,
		 attempts, last_error, abort_reason, done_at, created_at, updated_at
		 FROM aa_closeout_plans WHERE id=?`, id)
	  return scanPlan(row)
  }

  func (s *SQLStore) GetPlanByHopID(ctx context.Context, hopID string) (CloseoutPlan, bool, error) {
	  q := s.db.Conn(ctx)
	   row := q.QueryRowContext(ctx,
		  `SELECT id, hop_id, half_hop_id, state, selected_action, reason, actor,
		 attempts, last_error, abort_reason, done_at, created_at, updated_at
		 FROM aa_closeout_plans WHERE hop_id=? ORDER BY created_at DESC LIMIT 1`, hopID)
	  p, err := scanPlan(row)
	   if errors.Is(err, sql.ErrNoRows) || IsNotFound(err) {
		   return CloseoutPlan{}, false, nil
	  }
	  if err != nil {
		  return CloseoutPlan{}, false, err
	  }
	   return p, true, nil
   }

  func (s *SQLStore) ListPlans(ctx context.Context,
	  state PlanState, offset, limit int) ([]CloseoutPlan, error) {
	   if limit <= 0 || limit > 500 {
		   limit = 100
	  }
	   q := s.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx,
		   `SELECT id, hop_id, half_hop_id, state, selected_action, reason, actor,
		 attempts, last_error, abort_reason, done_at, created_at, updated_at
		 FROM aa_closeout_plans WHERE state=?
		 ORDER BY created_at ASC LIMIT ? OFFSET ?`, string(state), limit, offset)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   var out []CloseoutPlan
	   for rows.Next() {
		   p, err2 := scanPlan(rows)
		  if err2 != nil {
			   continue
		   }
		   out = append(out, p)
	  }
	  return out, rows.Err()
  }

// ── CloseoutLeg SQL ───────────────────────────────────────────────────────────

  func (s *SQLStore) PutLeg(ctx context.Context, l CloseoutLeg) error {
	  now := l.CreatedAt.UTC().Format(time.RFC3339Nano)
	  q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx,
		   `INSERT INTO aa_closeout_legs
		 (id, plan_id, hop_id, seq_no, action, state, idem_key,
		  entry_ref, note, attempts, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		   l.ID,
		  l.PlanID, l.HopID, l.SeqNo, string(l.Action), string(l.State),
		  l.IdemKey, l.EntryRef, l.Note, l.Attempts, now,
	  )
	   if err != nil && isDuplicate(err) {
		   return ErrIdempotent
	   }
	  return err
  }

  func (s *SQLStore) UpdateLeg(ctx context.Context, l CloseoutLeg) error {
	   var startedAt,
		  finishedAt *string
	  if l.StartedAt != nil {
		  t := l.StartedAt.UTC().Format(time.RFC3339Nano)
		   startedAt = &t
	   }
	   if l.FinishedAt != nil {
		   t := l.FinishedAt.UTC().Format(time.RFC3339Nano)
		   finishedAt = &t
	  }
	  q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx,
		  `UPDATE aa_closeout_legs SET
		 state=?, entry_ref=?, note=?, attempts=?, started_at=?, finished_at=?
		 WHERE id=?`,
		   string(l.State), l.EntryRef, l.Note, l.Attempts,
		   startedAt, finishedAt, l.ID,
	  )
	  return err
  }

   func (s *SQLStore) GetLeg(ctx context.Context, id string) (CloseoutLeg, error) {
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		   `SELECT id, plan_id, hop_id, seq_no, action, state, idem_key,
		 entry_ref, note, attempts, started_at, finished_at, created_at
		 FROM aa_closeout_legs WHERE id=?`, id)
	   return scanLeg(row)
  }

  func (s *SQLStore) GetLegByIdem(ctx context.Context, idemKey string) (CloseoutLeg, error) {
	   q := s.db.Conn(ctx)
	   row := q.QueryRowContext(ctx,
		   `SELECT id, plan_id, hop_id, seq_no, action, state, idem_key,
		 entry_ref, note, attempts, started_at, finished_at, created_at
		 FROM aa_closeout_legs WHERE idem_key=?`, idemKey)
	   return scanLeg(row)
  }

   func (s *SQLStore) ListLegs(ctx context.Context, planID string) ([]CloseoutLeg, error) {
	  q := s.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx,
		   `SELECT id, plan_id, hop_id, seq_no, action, state, idem_key,
		 entry_ref, note, attempts, started_at, finished_at, created_at
		 FROM aa_closeout_legs WHERE plan_id=?
		 ORDER BY seq_no ASC`, planID)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   var out []CloseoutLeg
	   for rows.Next() {
		   l, err2 := scanLeg(rows)
		   if err2 != nil {
			  continue
		   }
		   out = append(out, l)
	  }
	  return out, rows.Err()
   }

// ── XferSLA SQL ───────────────────────────────────────────────────────────────

   func (s *SQLStore) PutSLA(ctx context.Context, r XferSLA) error {
	  body, _ := json.Marshal(r)
	   now := time.Now().UTC().Format(time.RFC3339Nano)
	  q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx,
		  `INSERT INTO aa_xfer_slas (id, body, created_at, updated_at)
		 VALUES (?,?,?,?)`,
		   r.ID, body, now, now,
	  )
	  if err != nil && isDuplicate(err) {
		  return ErrConflict
	  }
	   return err
   }

   func (s *SQLStore) UpdateSLA(ctx context.Context, r XferSLA) error {
	  body, _ := json.Marshal(r)
	  now := time.Now().UTC().Format(time.RFC3339Nano)
	  q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx,
		   `UPDATE aa_xfer_slas SET body=?, updated_at=? WHERE id=?`,
		  body, now, r.ID,
	   )
	   return err
  }

  func (s *SQLStore) GetSLA(ctx context.Context, id string) (XferSLA, error) {
	   q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `SELECT body FROM aa_xfer_slas WHERE id=?`, id)
	  var body []byte
	  if err := row.Scan(&body); err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return XferSLA{}, ErrNotFound
		   }
		  return XferSLA{}, err
	  }
	  var r XferSLA
	   if err := json.Unmarshal(body, &r); err != nil {
		  return XferSLA{}, err
	   }
	  return r, nil
  }

   func (s *SQLStore) ListSLAs(ctx context.Context) ([]XferSLA, error) {
	   q := s.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, `SELECT body FROM aa_xfer_slas ORDER BY created_at ASC`)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   var out []XferSLA
	  for rows.Next() {
		  var body []byte
		  if err2 := rows.Scan(&body); err2 != nil {
			  continue
		   }
		  var r XferSLA
		   if json.Unmarshal(body, &r) == nil {
			  out = append(out, r)
		  }
	  }
	   return out, rows.Err()
   }

  func (s *SQLStore) DeleteSLA(ctx context.Context, id string) error {
	   q := s.db.Conn(ctx)
	   res, err := q.ExecContext(ctx, `DELETE FROM aa_xfer_slas WHERE id=?`, id)
	   if err != nil {
		  return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrNotFound
	  }
	  return nil
  }

// ── scan helpers ─────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

   func scanHop(row scanner) (HalfHop, error) {
	  var (
		   h           HalfHop
		  amtStr      string
		   slaDeadline string
		  detected string
		  updated     string
		  closedAt    sql.NullString
		  slaBreached int
		   xShard      int
	  )
	  err := row.Scan(
		   &h.ID, &h.HopID, &h.TransferID, &h.UserID, &h.Asset, &amtStr,
		   &h.FromBook, &h.ToBook, &h.Kind, &h.State,
		   &slaBreached,
		  &slaDeadline, &h.SLARuleID,
		  &h.CloseoutPlanID, &h.EscalateReason, &xShard,
		  &detected, &updated, &closedAt,
	  )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return HalfHop{}, ErrNotFound
		  }
		  return HalfHop{}, err
	  }
	  h.Amount,
		   _ = decimal.NewFromString(amtStr)
	   h.SLABreached = slaBreached != 0
	   h.XShard = xShard != 0
	   h.SLADeadline, _ = time.Parse(time.RFC3339Nano, slaDeadline)
	  h.DetectedAt, _ = time.Parse(time.RFC3339Nano, detected)
	  h.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	   if closedAt.Valid {
		   t, _ := time.Parse(time.RFC3339Nano, closedAt.String)
		  h.ClosedAt = &t
	   }
	  return h, nil
   }

  func scanHops(rows *sql.Rows) ([]HalfHop, error) {
	  var out []HalfHop
	   for rows.Next() {
		  h, err := scanHop(rows)
		  if err != nil {
			   continue
		  }
		  out = append(out, h)
	  }
	  return out, rows.Err()
  }

  func scanPlan(row scanner) (CloseoutPlan, error) {
	  var (
		   p CloseoutPlan
		   doneAt sql.NullString
		  created string
		   updated string
	  )
	   err := row.Scan(
		   &p.ID,
		  &p.HopID, &p.HalfHopID, &p.State, &p.SelectedAction,
		  &p.Reason, &p.Actor, &p.Attempts, &p.LastError, &p.AbortReason,
		   &doneAt, &created, &updated,
	  )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return CloseoutPlan{}, ErrNotFound
		   }
		  return CloseoutPlan{}, err
	   }
	   p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	  p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	   if doneAt.Valid {
		  t, _ := time.Parse(time.RFC3339Nano, doneAt.String)
		  p.DoneAt = &t
	   }
	  return p, nil
   }

   func scanLeg(row scanner) (CloseoutLeg, error) {
	  var (
		   l CloseoutLeg
		   startedAt sql.NullString
		   finishedAt sql.NullString
		  created    string
	   )
	  err := row.Scan(
		  &l.ID, &l.PlanID, &l.HopID, &l.SeqNo, &l.Action, &l.State,
		  &l.IdemKey, &l.EntryRef, &l.Note, &l.Attempts,
		  &startedAt, &finishedAt, &created,
	   )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return CloseoutLeg{}, ErrNotFound
		  }
		   return CloseoutLeg{}, err
	  }
	   l.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	  if startedAt.Valid {
		  t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		   l.StartedAt = &t
	  }
	   if finishedAt.Valid {
		   t, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
		   l.FinishedAt = &t
	  }
	  return l, nil
  }

  func boolInt(b bool) int {
	   if b {
		   return 1
	   }
	  return 0
  }

   func isDuplicate(err error) bool {
	   if err == nil {
		   return false
	  }
	  s := err.Error()
	  return strings.Contains(s, "Duplicate entry") ||
		   strings.Contains(s, "duplicate key") ||
		  strings.Contains(s, "UNIQUE constraint")
  }
