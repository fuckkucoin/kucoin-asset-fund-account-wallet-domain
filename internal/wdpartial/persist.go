package wdpartial

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ──────────────────────────────────────────────
// SQL 辅助
// ──────────────────────────────────────────────
// SQLDB 最小化数据库接口（兼容 *sql.DB 和 *sql.Tx）。

type SQLDB interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

  func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

  func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

  func scanTime(t *time.Time, v interface{}) {
	   if v == nil {
		  return
	   }
	  if tt, ok := v.(time.Time); ok {
		  *t = tt
	   }
  }

// ──────────────────────────────────────────────
// SQLHoldStore
// ──────────────────────────────────────────────
// SQLHoldStore Hold SQL 实现。
type SQLHoldStore struct {
	db SQLDB
}

// NewSQLHoldStore 构造 SQLHoldStore。

  func NewSQLHoldStore(db SQLDB) *SQLHoldStore {
	   return &SQLHoldStore{db: db}
  }

  func (s *SQLHoldStore) Create(ctx context.Context, h *HoldRecord) error {
	   const q = `INSERT INTO wdpartial_holds
(id, plan_id, user_id, asset, amount, seq, status, guard_gen_id, released_at, consumed_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE id=id`
	   _, err := s.db.ExecContext(ctx, q,
		  h.ID, h.PlanID, h.UserID, h.Asset, h.Amount, h.Seq, string(h.Status),
		  nullStr(h.GuardGenID), nullTime(h.ReleasedAt), nullTime(h.ConsumedAt),
		  h.CreatedAt, h.UpdatedAt,
	   )
	  return err
  }

  func (s *SQLHoldStore) Get(ctx context.Context, id string) (*HoldRecord, error) {
	   const q = `SELECT id, plan_id, user_id, asset, amount, seq, status, COALESCE(guard_gen_id,''),
released_at, consumed_at, created_at, updated_at
FROM wdpartial_holds WHERE id=? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, id)
	  h, err := scanHoldRow(row)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrHoldNotFound
		   }
		  return nil, err
	   }
	  return h, nil
  }

  func (s *SQLHoldStore) GetByPlan(ctx context.Context, planID string) ([]*HoldRecord, error) {
	  const q = `SELECT id, plan_id, user_id, asset, amount, seq, status, COALESCE(guard_gen_id,''),
released_at, consumed_at, created_at, updated_at
FROM wdpartial_holds WHERE plan_id=? ORDER BY seq`
	  rows, err := s.db.QueryContext(ctx, q, planID)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  return scanHoldRows(rows)
   }

   func (s *SQLHoldStore) ListOpenByPlan(ctx context.Context, planID string) ([]*HoldRecord, error) {
	  const q = `SELECT id, plan_id, user_id, asset, amount, seq, status, COALESCE(guard_gen_id,''),
released_at, consumed_at, created_at, updated_at
FROM wdpartial_holds WHERE plan_id=? AND status='open' ORDER BY seq`
	   rows, err := s.db.QueryContext(ctx, q, planID)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanHoldRows(rows)
   }

  func (s *SQLHoldStore) Update(ctx context.Context, h *HoldRecord) error {
	   const q = `UPDATE wdpartial_holds SET status=?, guard_gen_id=?, released_at=?, consumed_at=?, updated_at=? WHERE id=?`
	  res,
		  err := s.db.ExecContext(ctx, q,
		  string(h.Status), nullStr(h.GuardGenID), nullTime(h.ReleasedAt), nullTime(h.ConsumedAt),
		  h.UpdatedAt, h.ID,
	   )
	   if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrHoldNotFound
	  }
	   return nil
   }

   func scanHoldRow(row *sql.Row) (*HoldRecord, error) {
	  var h HoldRecord
	  var status string
	   var releasedAt,
		  consumedAt sql.NullTime
	  err := row.Scan(&h.ID, &h.PlanID, &h.UserID, &h.Asset, &h.Amount, &h.Seq,
		  &status, &h.GuardGenID, &releasedAt, &consumedAt, &h.CreatedAt, &h.UpdatedAt)
	  if err != nil {
		   return nil, err
	  }
	   h.Status = HoldStatus(status)
	   if releasedAt.Valid {
		   t := releasedAt.Time
		  h.ReleasedAt = &t
	  }
	   if consumedAt.Valid {
		   t := consumedAt.Time
		   h.ConsumedAt = &t
	  }
	   return &h, nil
   }

   func scanHoldRows(rows *sql.Rows) ([]*HoldRecord, error) {
	  var out []*HoldRecord
	  for rows.Next() {
		   var h HoldRecord
		   var status string
		  var releasedAt, consumedAt sql.NullTime
		   if err := rows.Scan(&h.ID, &h.PlanID, &h.UserID, &h.Asset, &h.Amount, &h.Seq,
			  &status, &h.GuardGenID, &releasedAt, &consumedAt, &h.CreatedAt, &h.UpdatedAt); err != nil {
			  return nil, err
		   }
		   h.Status = HoldStatus(status)
		  if releasedAt.Valid {
			  t := releasedAt.Time
			  h.ReleasedAt = &t
		   }
		   if consumedAt.Valid {
			  t := consumedAt.Time
			  h.ConsumedAt = &t
		   }
		   out = append(out, &h)
	   }
	   return out, rows.Err()
   }

// ──────────────────────────────────────────────
// SQLFillStore
// ──────────────────────────────────────────────
// SQLFillStore Fill SQL 实现。

type SQLFillStore struct {
	db SQLDB
}

// NewSQLFillStore 构造 SQLFillStore。
   func NewSQLFillStore(db SQLDB) *SQLFillStore {
	   return &SQLFillStore{db: db}
  }

  func (s *SQLFillStore) Create(ctx context.Context, f *FillRecord) error {
	  const q = `INSERT INTO wdpartial_fills
(id, plan_id, hold_id, user_id, asset, amount, seq, status, mutex_token_id, confirmed_at, rejected_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE id=id`
	  _, err := s.db.ExecContext(ctx, q,
		  f.ID, f.PlanID, f.HoldID, f.UserID, f.Asset, f.Amount, f.Seq, string(f.Status),
		   nullStr(f.MutexTokenID), nullTime(f.ConfirmedAt), nullTime(f.RejectedAt),
		  f.CreatedAt, f.UpdatedAt,
	  )
	   return err
   }

   func (s *SQLFillStore) Get(ctx context.Context, id string) (*FillRecord, error) {
	  const q = `SELECT id, plan_id, hold_id, user_id, asset, amount, seq, status,
COALESCE(mutex_token_id,''), confirmed_at, rejected_at, created_at, updated_at
FROM wdpartial_fills WHERE id=? LIMIT 1`
	  row := s.db.QueryRowContext(ctx,
		   q, id)
	   f, err := scanFillRow(row)
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrFillNotFound
		   }
		   return nil, err
	   }
	   return f, nil
  }

  func (s *SQLFillStore) GetByPlan(ctx context.Context, planID string) ([]*FillRecord, error) {
	  const q = `SELECT id, plan_id, hold_id, user_id, asset, amount, seq, status,
COALESCE(mutex_token_id,''), confirmed_at, rejected_at, created_at, updated_at
FROM wdpartial_fills WHERE plan_id=? ORDER BY seq`
	   rows, err := s.db.QueryContext(ctx, q, planID)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanFillRows(rows)
   }

  func (s *SQLFillStore) ListConfirmedByPlan(ctx context.Context, planID string) ([]*FillRecord, error) {
	  const q = `SELECT id, plan_id, hold_id, user_id, asset, amount, seq, status,
COALESCE(mutex_token_id,''), confirmed_at, rejected_at, created_at, updated_at
FROM wdpartial_fills WHERE plan_id=? AND status='confirmed' ORDER BY seq`
	  rows, err := s.db.QueryContext(ctx, q, planID)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanFillRows(rows)
  }

  func (s *SQLFillStore) Update(ctx context.Context, f *FillRecord) error {
	   const q = `UPDATE wdpartial_fills SET status=?, mutex_token_id=?, confirmed_at=?, rejected_at=?, updated_at=? WHERE id=?`
	  res, err := s.db.ExecContext(ctx, q,
		   string(f.Status), nullStr(f.MutexTokenID), nullTime(f.ConfirmedAt), nullTime(f.RejectedAt),
		   f.UpdatedAt, f.ID,
	   )
	  if err != nil {
		   return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrFillNotFound
	   }
	  return nil
  }

   func scanFillRow(row *sql.Row) (*FillRecord, error) {
	  var f FillRecord
	  var status string
	  var confirmedAt, rejectedAt sql.NullTime
	  err := row.Scan(&f.ID, &f.PlanID, &f.HoldID, &f.UserID, &f.Asset, &f.Amount, &f.Seq,
		   &status,
		  &f.MutexTokenID, &confirmedAt, &rejectedAt, &f.CreatedAt, &f.UpdatedAt)
	   if err != nil {
		  return nil, err
	   }
	   f.Status = FillStatus(status)
	  if confirmedAt.Valid {
		   t := confirmedAt.Time
		   f.ConfirmedAt = &t
	  }
	  if rejectedAt.Valid {
		  t := rejectedAt.Time
		  f.RejectedAt = &t
	  }
	   return &f, nil
   }

  func scanFillRows(rows *sql.Rows) ([]*FillRecord, error) {
	  var out []*FillRecord
	   for rows.Next() {
		  var f FillRecord
		   var status string
		   var confirmedAt, rejectedAt sql.NullTime
		  if err := rows.Scan(&f.ID,
			   &f.PlanID, &f.HoldID, &f.UserID, &f.Asset, &f.Amount, &f.Seq,
			   &status, &f.MutexTokenID, &confirmedAt, &rejectedAt, &f.CreatedAt, &f.UpdatedAt); err != nil {
			   return nil, err
		   }
		  f.Status = FillStatus(status)
		   if confirmedAt.Valid {
			  t := confirmedAt.Time
			   f.ConfirmedAt = &t
		   }
		   if rejectedAt.Valid {
			   t := rejectedAt.Time
			  f.RejectedAt = &t
		   }
		  out = append(out, &f)
	   }
	  return out, rows.Err()
  }

// ──────────────────────────────────────────────
// SQLLeaseStore
// ──────────────────────────────────────────────
// SQLLeaseStore RemReleaseLease SQL 实现。
type SQLLeaseStore struct {
	db SQLDB
}

// NewSQLLeaseStore 构造 SQLLeaseStore。

  func NewSQLLeaseStore(db SQLDB) *SQLLeaseStore {
	   return &SQLLeaseStore{db: db}
  }

  func (s *SQLLeaseStore) Create(ctx context.Context, l *RemReleaseLease) error {
	  const q = `INSERT INTO wdpartial_rem_leases
(id, plan_id, user_id, generation, status, blocked_by_fill_id, expires_at, releasing_at, done_at, revoked_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE id=id`
	  _, err := s.db.ExecContext(ctx, q,
		  l.ID,
		   l.PlanID, l.UserID, l.Generation, string(l.Status),
		   nullStr(l.BlockedByFillID),
		   l.ExpiresAt, nullTime(l.ReleasingAt),
		   nullTime(l.DoneAt), nullTime(l.RevokedAt), l.CreatedAt, l.UpdatedAt,
	   )
	   return err
   }

  func (s *SQLLeaseStore) Get(ctx context.Context, id string) (*RemReleaseLease, error) {
	  const q = `SELECT id, plan_id, user_id, generation, status, COALESCE(blocked_by_fill_id,''), 
expires_at, releasing_at, done_at, revoked_at, created_at, updated_at
FROM wdpartial_rem_leases WHERE id=? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, id)
	   return scanLeaseRow(row)
   }

// GetOpenByPlan 使用 FOR UPDATE 行锁防止并发颁发重复 open 租约。
   func (s *SQLLeaseStore) GetOpenByPlan(ctx context.Context, planID string) (*RemReleaseLease, error) {
	   const q = `SELECT id, plan_id, user_id, generation, status, COALESCE(blocked_by_fill_id,''),
expires_at, releasing_at, done_at, revoked_at, created_at, updated_at
FROM wdpartial_rem_leases WHERE plan_id=? AND status='open' LIMIT 1 FOR UPDATE`
	  row := s.db.QueryRowContext(ctx, q, planID)
	   l, err := scanLeaseRow(row)
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrLeaseNotFound
		  }
		  return nil, err
	  }
	   return l, nil
   }

   func (s *SQLLeaseStore) Update(ctx context.Context, l *RemReleaseLease) error {
	  const q = `UPDATE wdpartial_rem_leases SET status=?, blocked_by_fill_id=?,
releasing_at=?, done_at=?, revoked_at=?, updated_at=? WHERE id=?`
	  res, err := s.db.ExecContext(ctx, q,
		  string(l.Status),
		   nullStr(l.BlockedByFillID),
		   nullTime(l.ReleasingAt), nullTime(l.DoneAt), nullTime(l.RevokedAt),
		  l.UpdatedAt, l.ID,
	   )
	  if err != nil {
		  return err
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrLeaseNotFound
	  }
	   return nil
   }

  func (s *SQLLeaseStore) ListExpired(ctx context.Context, limit int) ([]*RemReleaseLease, error) {
	  q := fmt.Sprintf(`SELECT id, plan_id, user_id, generation, status, COALESCE(blocked_by_fill_id,''),
expires_at, releasing_at, done_at, revoked_at, created_at, updated_at
FROM wdpartial_rem_leases WHERE status NOT IN ('done','revoked') AND expires_at < NOW() LIMIT %d`, limit)
	   rows,
		  err := s.db.QueryContext(ctx, q)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanLeaseRows(rows)
  }

  func scanLeaseRow(row *sql.Row) (*RemReleaseLease, error) {
	  var l RemReleaseLease
	  var status string
	  var releasingAt, doneAt, revokedAt sql.NullTime
	   err := row.Scan(&l.ID, &l.PlanID, &l.UserID, &l.Generation, &status, &l.BlockedByFillID,
		  &l.ExpiresAt, &releasingAt, &doneAt, &revokedAt, &l.CreatedAt, &l.UpdatedAt)
	  if err != nil {
		   return nil, err
	   }
	  l.Status = LeaseStatus(status)
	   if releasingAt.Valid {
		  t := releasingAt.Time
		  l.ReleasingAt = &t
	   }
	   if doneAt.Valid {
		  t := doneAt.Time
		   l.DoneAt = &t
	   }
	  if revokedAt.Valid {
		  t := revokedAt.Time
		  l.RevokedAt = &t
	   }
	   return &l, nil
  }

  func scanLeaseRows(rows *sql.Rows) ([]*RemReleaseLease, error) {
	   var out []*RemReleaseLease
	  for rows.Next() {
		   var l RemReleaseLease
		   var status string
		  var releasingAt,
			  doneAt, revokedAt sql.NullTime
		  if err := rows.Scan(&l.ID, &l.PlanID, &l.UserID, &l.Generation, &status, &l.BlockedByFillID,
			  &l.ExpiresAt, &releasingAt, &doneAt, &revokedAt, &l.CreatedAt, &l.UpdatedAt); err != nil {
			  return nil, err
		   }
		  l.Status = LeaseStatus(status)
		  if releasingAt.Valid {
			  t := releasingAt.Time
			  l.ReleasingAt = &t
		   }
		  if doneAt.Valid {
			   t := doneAt.Time
			  l.DoneAt = &t
		  }
		   if revokedAt.Valid {
			  t := revokedAt.Time
			  l.RevokedAt = &t
		   }
		   out = append(out, &l)
	   }
	   return out, rows.Err()
   }

// ──────────────────────────────────────────────
// SQLConserveCaseStore
// ──────────────────────────────────────────────
// SQLConserveCaseStore ConserveCase SQL 实现。
type SQLConserveCaseStore struct {
	db SQLDB
}

// NewSQLConserveCaseStore 构造 SQLConserveCaseStore。

   func NewSQLConserveCaseStore(db SQLDB) *SQLConserveCaseStore {
	  return &SQLConserveCaseStore{db: db}
   }

  func (s *SQLConserveCaseStore) Create(ctx context.Context, c *ConserveCase) error {
	  const q = `INSERT INTO wdpartial_conserve_cases
(id, plan_id, user_id, status, conserve_snapshot, fail_reason, proof_payload, approver, approver2,
detected_at, proven_at, closed_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE id=id`
	   _, err := s.db.ExecContext(ctx, q,
		   c.ID, c.PlanID, c.UserID, string(c.Status),
		   c.ConserveSnapshot, c.FailReason,
		   nullStr(c.ProofPayload), nullStr(c.Approver), nullStr(c.Approver2),
		   c.DetectedAt, nullTime(c.ProvenAt), nullTime(c.ClosedAt), c.UpdatedAt,
	  )
	  return err
   }

  func (s *SQLConserveCaseStore) Get(ctx context.Context, id string) (*ConserveCase, error) {
	   const q = `SELECT id, plan_id, user_id, status, conserve_snapshot, fail_reason,
COALESCE(proof_payload,''), COALESCE(approver,''), COALESCE(approver2,''),
detected_at, proven_at, closed_at, updated_at
FROM wdpartial_conserve_cases WHERE id=? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, id)
	  return scanConserveCaseRow(row)
   }

  func (s *SQLConserveCaseStore) GetOpenByPlan(ctx context.Context, planID string) (*ConserveCase, error) {
	  const q = `SELECT id, plan_id, user_id, status, conserve_snapshot, fail_reason,
COALESCE(proof_payload,''), COALESCE(approver,''), COALESCE(approver2,''),
detected_at, proven_at, closed_at, updated_at
FROM wdpartial_conserve_cases WHERE plan_id=? AND status='open' LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, planID)
	  c, err := scanConserveCaseRow(row)
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrConserveCaseNotFound
		  }
		   return nil, err
	   }
	   return c, nil
  }

// Update 更新守恒案件状态。conserve_snapshot 字段禁止更新（SQL 语句不含该字段）。

  func (s *SQLConserveCaseStore) Update(ctx context.Context, c *ConserveCase) error {
	   const q = `UPDATE wdpartial_conserve_cases SET status=?, proof_payload=?, approver=?, approver2=?,
proven_at=?, closed_at=?, updated_at=? WHERE id=?`
	   res, err := s.db.ExecContext(ctx, q,
		   string(c.Status), nullStr(c.ProofPayload), nullStr(c.Approver), nullStr(c.Approver2),
		  nullTime(c.ProvenAt),
		   nullTime(c.ClosedAt), c.UpdatedAt, c.ID,
	   )
	   if err != nil {
		   return err
	   }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrConserveCaseNotFound
	  }
	  return nil
  }

   func (s *SQLConserveCaseStore) ListOpen(ctx context.Context, limit int) ([]*ConserveCase, error) {
	   q := fmt.Sprintf(`SELECT id, plan_id, user_id, status, conserve_snapshot, fail_reason,
COALESCE(proof_payload,''), COALESCE(approver,''), COALESCE(approver2,''),
detected_at, proven_at, closed_at, updated_at
FROM wdpartial_conserve_cases WHERE status='open' ORDER BY detected_at DESC LIMIT %d`, limit)
	   rows, err := s.db.QueryContext(ctx, q)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanConserveCaseRows(rows)
  }

   func scanConserveCaseRow(row *sql.Row) (*ConserveCase, error) {
	   var c ConserveCase
	   var status string
	   var provenAt, closedAt sql.NullTime
	  err := row.Scan(&c.ID, &c.PlanID, &c.UserID, &status,
		  &c.ConserveSnapshot, &c.FailReason, &c.ProofPayload, &c.Approver, &c.Approver2,
		   &c.DetectedAt,
		   &provenAt, &closedAt, &c.UpdatedAt)
	  if err != nil {
		  return nil, err
	  }
	  c.Status = ConserveCaseStatus(status)
	   if provenAt.Valid {
		   t := provenAt.Time
		   c.ProvenAt = &t
	   }
	  if closedAt.Valid {
		  t := closedAt.Time
		   c.ClosedAt = &t
	  }
	   return &c, nil
  }

   func scanConserveCaseRows(rows *sql.Rows) ([]*ConserveCase, error) {
	   var out []*ConserveCase
	  for rows.Next() {
		   var c ConserveCase
		  var status string
		   var provenAt, closedAt sql.NullTime
		   if err := rows.Scan(&c.ID, &c.PlanID, &c.UserID, &status,
			   &c.ConserveSnapshot,
			  &c.FailReason, &c.ProofPayload, &c.Approver, &c.Approver2,
			   &c.DetectedAt, &provenAt, &closedAt, &c.UpdatedAt); err != nil {
			  return nil, err
		   }
		   c.Status = ConserveCaseStatus(status)
		   if provenAt.Valid {
			  t := provenAt.Time
			   c.ProvenAt = &t
		   }
		  if closedAt.Valid {
			   t := closedAt.Time
			  c.ClosedAt = &t
		  }
		  out = append(out, &c)
	   }
	  return out, rows.Err()
   }

// ──────────────────────────────────────────────
// SQLGuardGenStore
// ──────────────────────────────────────────────
// SQLGuardGenStore BroadcastGuardGen SQL 实现。
type SQLGuardGenStore struct {
	db SQLDB
}

// NewSQLGuardGenStore 构造 SQLGuardGenStore。

   func NewSQLGuardGenStore(db SQLDB) *SQLGuardGenStore {
	   return &SQLGuardGenStore{db: db}
  }

  func (s *SQLGuardGenStore) Create(ctx context.Context, g *BroadcastGuardGen) error {
	  const q = `INSERT INTO wdpartial_guard_gens
(id, plan_id, live_hash, generation, status, invalidated_reason, invalidated_at, consumed_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE id=id`
	   _, err := s.db.ExecContext(ctx, q,
		   g.ID, g.PlanID, g.LiveHash, g.Generation, string(g.Status),
		  nullStr(g.InvalidatedReason), nullTime(g.InvalidatedAt), nullTime(g.ConsumedAt),
		  g.CreatedAt, g.UpdatedAt,
	   )
	   return err
  }

   func (s *SQLGuardGenStore) Get(ctx context.Context, id string) (*BroadcastGuardGen, error) {
	   const q = `SELECT id, plan_id, live_hash, generation, status,
COALESCE(invalidated_reason,''), invalidated_at, consumed_at, created_at, updated_at
FROM wdpartial_guard_gens WHERE id=? LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, id)
	  g, err := scanGuardGenRow(row)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrGuardGenNotFound
		  }
		   return nil, err
	   }
	   return g, nil
  }

   func (s *SQLGuardGenStore) GetActiveByPlan(ctx context.Context, planID string) (*BroadcastGuardGen, error) {
	   const q = `SELECT id, plan_id, live_hash, generation, status,
COALESCE(invalidated_reason,''), invalidated_at, consumed_at, created_at, updated_at
FROM wdpartial_guard_gens WHERE plan_id=? AND status='active' LIMIT 1`
	  row := s.db.QueryRowContext(ctx,
		  q, planID)
	  g, err := scanGuardGenRow(row)
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrGuardGenNotFound
		   }
		  return nil, err
	   }
	  return g, nil
  }

   func (s *SQLGuardGenStore) Update(ctx context.Context, g *BroadcastGuardGen) error {
	  const q = `UPDATE wdpartial_guard_gens SET status=?, invalidated_reason=?, invalidated_at=?, consumed_at=?, updated_at=? WHERE id=?`
	  res, err := s.db.ExecContext(ctx, q,
		  string(g.Status), nullStr(g.InvalidatedReason), nullTime(g.InvalidatedAt),
		   nullTime(g.ConsumedAt), g.UpdatedAt, g.ID,
	   )
	  if err != nil {
		   return err
	   }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrGuardGenNotFound
	  }
	   return nil
  }

  func (s *SQLGuardGenStore) ListActive(ctx context.Context, limit int) ([]*BroadcastGuardGen, error) {
	   q := fmt.Sprintf(`SELECT id, plan_id, live_hash, generation, status,
COALESCE(invalidated_reason,''), invalidated_at, consumed_at, created_at, updated_at
FROM wdpartial_guard_gens WHERE status='active' ORDER BY created_at LIMIT %d`, limit)
	  rows,
		  err := s.db.QueryContext(ctx, q)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanGuardGenRows(rows)
  }

  func scanGuardGenRow(row *sql.Row) (*BroadcastGuardGen, error) {
	  var g BroadcastGuardGen
	  var status string
	   var invalidatedAt, consumedAt sql.NullTime
	   err := row.Scan(&g.ID,
		   &g.PlanID, &g.LiveHash, &g.Generation, &status,
		   &g.InvalidatedReason, &invalidatedAt, &consumedAt, &g.CreatedAt, &g.UpdatedAt)
	   if err != nil {
		   return nil, err
	  }
	   g.Status = GuardGenStatus(status)
	  if invalidatedAt.Valid {
		  t := invalidatedAt.Time
		   g.InvalidatedAt = &t
	  }
	  if consumedAt.Valid {
		   t := consumedAt.Time
		  g.ConsumedAt = &t
	  }
	  return &g, nil
   }

   func scanGuardGenRows(rows *sql.Rows) ([]*BroadcastGuardGen, error) {
	  var out []*BroadcastGuardGen
	   for rows.Next() {
		  var g BroadcastGuardGen
		  var status string
		   var invalidatedAt, consumedAt sql.NullTime
		   if err := rows.Scan(&g.ID, &g.PlanID, &g.LiveHash, &g.Generation, &status,
			   &g.InvalidatedReason, &invalidatedAt, &consumedAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
			   return nil, err
		  }
		   g.Status = GuardGenStatus(status)
		   if invalidatedAt.Valid {
			  t := invalidatedAt.Time
			   g.InvalidatedAt = &t
		  }
		   if consumedAt.Valid {
			   t := consumedAt.Time
			  g.ConsumedAt = &t
		  }
		   out = append(out, &g)
	  }
	  return out, rows.Err()
   }

// ──────────────────────────────────────────────
// 占位符（暂不使用，消除 import 警告）
// ──────────────────────────────────────────────

var _ = scanTime
