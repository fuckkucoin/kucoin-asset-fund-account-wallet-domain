// - 依赖 *sql.DB；不持有连接池，由外部注入。
// - ConsumeGen CAS：UPDATE WHERE consumed=false；RowsAffected=0 返回 ErrGenConsumed。
// - SaveLease：唯一索引 uq_lo_order_id 冲突返回 ErrGenAlreadyExists。
// - UpdateLeaseState CAS：UPDATE WHERE state=from；RowsAffected=0 返回 ErrInvalidTransition。
// - SaveDupProof / SaveResidueProof：唯一索引冲突返回 ErrAlreadyProven。
// - 无 DELETE 操作；所有状态变更只通过 UPDATE SET state/consumed。
// 不变量：
// - 表名与 migrations/0122_aa_lendoccupygate.sql 一致。
// - 时间字段均为 UTC DATETIME(3)。
// - isDup 判断 MySQL/TiDB Error 1062。

// Author: Knox
// Date: 2021.08.22

package lendoccupygate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// SQLStore 基于 *sql.DB 的 Store 实现（MySQL/TiDB）。
type SQLStore struct {
	db    *sql.DB
	clock func() time.Time
}

// NewSQLStore 构造 SQLStore。
  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{
		   db:    db,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithSQLClock 注入时钟。

  func (s *SQLStore) WithSQLClock(fn func() time.Time) *SQLStore {
	   if fn != nil {
		   s.clock = fn
	   }
	  return s
   }

  // isDup 判断 MySQL/TiDB 唯一索引冲突（Error 1062）。
  func isDup(err error) bool {
	   if err == nil {
		  return false
	  }
	  msg := err.Error()
	  return strings.Contains(msg, "Duplicate entry") ||
		   strings.Contains(msg, "duplicate key") ||
		  strings.Contains(msg, "1062")
  }

// nullTime 将 *time.Time 转为 interface{}（nil → SQL NULL）。

  func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

// ── LendGen ──────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveGen(ctx context.Context, g *LendGen) error {
	  if s == nil || g == nil {
		  return ErrUnavailable
	   }
	   const q = `INSERT IGNORE INTO lend_gens
		(gen_id, order_id, user_id, asset, amount, ledger_idem_key, consumed, consumed_by, consumed_at, issued_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	   _,
		  err := s.db.ExecContext(ctx, q,
		   g.GenID, g.OrderID, g.UserID, g.Asset, g.Amount, g.LedgerIdemKey,
		   g.Consumed, g.ConsumedBy, nullTime(g.ConsumedAt),
		  g.IssuedAt, g.CreatedAt,
	  )
	  return err
   }

   func (s *SQLStore) GetGenByOrder(ctx context.Context, orderID string) (*LendGen, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  const q = `SELECT gen_id, order_id, user_id, asset, amount, ledger_idem_key,
		consumed, consumed_by, consumed_at, issued_at, created_at
		FROM lend_gens WHERE order_id = ? LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, orderID)
	  return scanGen(row)
   }

   func (s *SQLStore) GetGen(ctx context.Context, genID string) (*LendGen, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  const q = `SELECT gen_id, order_id, user_id, asset, amount, ledger_idem_key,
		consumed, consumed_by, consumed_at, issued_at, created_at
		FROM lend_gens WHERE gen_id = ? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, genID)
	  return scanGen(row)
   }

   func scanGen(row *sql.Row) (*LendGen, error) {
	   var g LendGen
	   var consumedBy sql.NullString
	   var consumedAt sql.NullTime
	  err := row.Scan(
		   &g.GenID,
		  &g.OrderID, &g.UserID, &g.Asset, &g.Amount, &g.LedgerIdemKey,
		   &g.Consumed, &consumedBy, &consumedAt,
		  &g.IssuedAt, &g.CreatedAt,
	   )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrGenNotFound
		   }
		  return nil, err
	  }
	  if consumedBy.Valid {
		  g.ConsumedBy = consumedBy.String
	   }
	  if consumedAt.Valid {
		  t := consumedAt.Time
		   g.ConsumedAt = &t
	  }
	   return &g, nil
  }

  func (s *SQLStore) ConsumeGen(ctx context.Context, genID string, consumedBy string, at time.Time) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   const q = `UPDATE lend_gens SET consumed=1, consumed_by=?, consumed_at=?
		WHERE gen_id=? AND consumed=0`
	  res, err := s.db.ExecContext(ctx, q, consumedBy, at, genID)
	  if err != nil {
		   return err
	  }
	  n, err := res.RowsAffected()
	  if err != nil {
		   return err
	  }
	  if n == 0 {
		   // 0 行受影响：gen 已消费或不存在
		   g, getErr := s.GetGen(ctx, genID)
		   if getErr != nil {
			   return ErrGenNotFound
		  }
		  if g.Consumed {
			   return ErrGenConsumed
		   }
		   return ErrGenNotFound
	   }
	  return nil
   }

// ── LendOccupyLease ──────────────────────────────────────────────────────────

   func (s *SQLStore) SaveLease(ctx context.Context, l *LendOccupyLease) error {
	   if s == nil || l == nil {
		  return ErrUnavailable
	  }
	  const q = `INSERT INTO lend_occupies
		(lease_id, gen_id, order_id, user_id, asset, amount, state, ledger_idem_key, hold_ref,
		 placed_at, settled_at, cancelled_at, residue_closed_at, residue_proof_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		   l.LeaseID, l.GenID, l.OrderID, l.UserID, l.Asset, l.Amount,
		  string(l.State), l.LedgerIdemKey, l.HoldRef,
		   l.PlacedAt,
		   nullTime(l.SettledAt), nullTime(l.CancelledAt), nullTime(l.ResidueClosedAt),
		  l.ResidueProofID, l.CreatedAt, l.UpdatedAt,
	   )
	  if err != nil {
		  if isDup(err) {
			  return ErrGenAlreadyExists
		  }
		  return err
	   }
	   return nil
   }

   func (s *SQLStore) GetLeaseByOrder(ctx context.Context, orderID string) (*LendOccupyLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   const q = `SELECT lease_id, gen_id, order_id, user_id, asset, amount, state,
		ledger_idem_key, hold_ref, placed_at, settled_at, cancelled_at, residue_closed_at,
		residue_proof_id, created_at, updated_at
		FROM lend_occupies WHERE order_id=? LIMIT 1`
	   row := s.db.QueryRowContext(ctx,
		   q, orderID)
	   return scanLease(row)
  }

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*LendOccupyLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  const q = `SELECT lease_id, gen_id, order_id, user_id, asset, amount, state,
		ledger_idem_key, hold_ref, placed_at, settled_at, cancelled_at, residue_closed_at,
		residue_proof_id, created_at, updated_at
		FROM lend_occupies WHERE lease_id=? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, leaseID)
	   return scanLease(row)
  }

   func scanLease(row *sql.Row) (*LendOccupyLease, error) {
	   var l LendOccupyLease
	  var settledAt, cancelledAt, residueClosedAt sql.NullTime
	   var residueProofID sql.NullString
	  err := row.Scan(
		  &l.LeaseID,
		   &l.GenID, &l.OrderID, &l.UserID, &l.Asset, &l.Amount, &l.State,
		  &l.LedgerIdemKey, &l.HoldRef,
		  &l.PlacedAt, &settledAt, &cancelledAt, &residueClosedAt,
		   &residueProofID, &l.CreatedAt, &l.UpdatedAt,
	  )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrLeaseNotFound
		  }
		   return nil, err
	   }
	  if settledAt.Valid {
		   t := settledAt.Time
		  l.SettledAt = &t
	  }
	  if cancelledAt.Valid {
		   t := cancelledAt.Time
		   l.CancelledAt = &t
	  }
	   if residueClosedAt.Valid {
		  t := residueClosedAt.Time
		   l.ResidueClosedAt = &t
	   }
	  if residueProofID.Valid {
		  l.ResidueProofID = residueProofID.String
	   }
	   return &l, nil
  }

  func (s *SQLStore) UpdateLeaseState(ctx context.Context, leaseID string, from, to LeaseState, patch *LendOccupyLease) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  now := s.clock()
	   var settledAt, cancelledAt, residueClosedAt interface{}
	  residueProofID := ""
	   if patch != nil {
		   settledAt = nullTime(patch.SettledAt)
		  cancelledAt = nullTime(patch.CancelledAt)
		  residueClosedAt = nullTime(patch.ResidueClosedAt)
		  residueProofID = patch.ResidueProofID
	   }
	  const q = `UPDATE lend_occupies SET state=?, settled_at=COALESCE(?,settled_at),
		cancelled_at=COALESCE(?,cancelled_at), residue_closed_at=COALESCE(?,residue_closed_at),
		residue_proof_id=COALESCE(NULLIF(?,''),residue_proof_id), updated_at=?
		WHERE lease_id=? AND state=?`
	   res, err := s.db.ExecContext(ctx, q,
		  string(to), settledAt, cancelledAt, residueClosedAt, residueProofID, now,
		  leaseID, string(from),
	   )
	  if err != nil {
		   return err
	  }
	  n, err := res.RowsAffected()
	   if err != nil {
		  return err
	   }
	   if n == 0 {
		  return ErrInvalidTransition
	   }
	  return nil
  }

   func (s *SQLStore) ListStalePlacedLeases(ctx context.Context, staleThreshold time.Time, limit int) ([]*LendOccupyLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 200
	  }
	   const q = `SELECT lease_id, gen_id, order_id, user_id, asset, amount, state,
		ledger_idem_key, hold_ref, placed_at, settled_at, cancelled_at, residue_closed_at,
		residue_proof_id, created_at, updated_at
		FROM lend_occupies WHERE state='placed' AND placed_at < ?
		ORDER BY placed_at ASC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, staleThreshold, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanLeaseRows(rows)
  }

   func (s *SQLStore) ListLeasesByUser(ctx context.Context, userID string, limit int) ([]*LendOccupyLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	  const q = `SELECT lease_id, gen_id, order_id, user_id, asset, amount, state,
		ledger_idem_key, hold_ref, placed_at, settled_at, cancelled_at, residue_closed_at,
		residue_proof_id, created_at, updated_at
		FROM lend_occupies WHERE user_id=?
		ORDER BY placed_at DESC LIMIT ?`
	   rows,
		   err := s.db.QueryContext(ctx, q, userID, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanLeaseRows(rows)
   }

  func scanLeaseRows(rows *sql.Rows) ([]*LendOccupyLease, error) {
	  var out []*LendOccupyLease
	   for rows.Next() {
		   var l LendOccupyLease
		  var settledAt,
			  cancelledAt, residueClosedAt sql.NullTime
		  var residueProofID sql.NullString
		  err := rows.Scan(
			   &l.LeaseID, &l.GenID, &l.OrderID, &l.UserID, &l.Asset, &l.Amount, &l.State,
			  &l.LedgerIdemKey, &l.HoldRef,
			  &l.PlacedAt, &settledAt, &cancelledAt, &residueClosedAt,
			  &residueProofID, &l.CreatedAt, &l.UpdatedAt,
		  )
		  if err != nil {
			   return nil, err
		   }
		  if settledAt.Valid {
			  t := settledAt.Time
			   l.SettledAt = &t
		  }
		  if cancelledAt.Valid {
			   t := cancelledAt.Time
			  l.CancelledAt = &t
		   }
		   if residueClosedAt.Valid {
			  t := residueClosedAt.Time
			   l.ResidueClosedAt = &t
		   }
		  if residueProofID.Valid {
			   l.ResidueProofID = residueProofID.String
		   }
		  out = append(out, &l)
	  }
	   return out, rows.Err()
   }

// ── LendDupReleaseProof ───────────────────────────────────────────────────────

   func (s *SQLStore) SaveDupProof(ctx context.Context, p *LendDupReleaseProof) error {
	   if s == nil || p == nil {
		   return ErrUnavailable
	   }
	  const q = `INSERT INTO lend_proofs
		(proof_id, proof_type, gen_id, order_id, user_id, asset, amount, kind,
		 first_consumed_by, first_consumed_at, attempted_by, attempted_at, created_at)
		VALUES (?, 'dup', ?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q,
		   p.ProofID, p.GenID, p.OrderID, p.UserID, p.Asset, p.Amount,
		  string(p.Kind), p.FirstConsumedBy, p.FirstConsumedAt, p.AttemptedBy, p.AttemptedAt, p.CreatedAt,
	   )
	  if err != nil {
		  if isDup(err) {
			   return ErrAlreadyProven
		  }
		  return err
	  }
	  return nil
  }

  func (s *SQLStore) GetDupProofByGen(ctx context.Context, genID string) (*LendDupReleaseProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  const q = `SELECT proof_id, gen_id, order_id, user_id, asset, amount, kind,
		first_consumed_by, first_consumed_at, attempted_by, attempted_at, created_at
		FROM lend_proofs WHERE gen_id=? AND proof_type='dup' LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, genID)
	   return scanDupProof(row)
   }

  func (s *SQLStore) GetDupProof(ctx context.Context, proofID string) (*LendDupReleaseProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   const q = `SELECT proof_id, gen_id, order_id, user_id, asset, amount, kind,
		first_consumed_by, first_consumed_at, attempted_by, attempted_at, created_at
		FROM lend_proofs WHERE proof_id=? AND proof_type='dup' LIMIT 1`
	  row := s.db.QueryRowContext(ctx,
		  q, proofID)
	  return scanDupProof(row)
   }

   func scanDupProof(row *sql.Row) (*LendDupReleaseProof, error) {
	   var p LendDupReleaseProof
	   err := row.Scan(
		   &p.ProofID, &p.GenID, &p.OrderID, &p.UserID, &p.Asset, &p.Amount, &p.Kind,
		  &p.FirstConsumedBy,
		  &p.FirstConsumedAt, &p.AttemptedBy, &p.AttemptedAt, &p.CreatedAt,
	  )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrDupProofNotFound
		   }
		   return nil, err
	   }
	   return &p, nil
  }

  func (s *SQLStore) ListDupProofsByOrder(ctx context.Context, orderID string) ([]*LendDupReleaseProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   const q = `SELECT proof_id, gen_id, order_id, user_id, asset, amount, kind,
		first_consumed_by, first_consumed_at, attempted_by, attempted_at, created_at
		FROM lend_proofs WHERE order_id=? AND proof_type='dup' ORDER BY created_at ASC`
	  rows, err := s.db.QueryContext(ctx, q, orderID)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []*LendDupReleaseProof
	  for rows.Next() {
		  var p LendDupReleaseProof
		   if err := rows.Scan(
			   &p.ProofID, &p.GenID, &p.OrderID, &p.UserID, &p.Asset, &p.Amount, &p.Kind,
			   &p.FirstConsumedBy, &p.FirstConsumedAt, &p.AttemptedBy, &p.AttemptedAt, &p.CreatedAt,
		   ); err != nil {
			  return nil, err
		  }
		   out = append(out, &p)
	   }
	   return out, rows.Err()
  }

// ── LendResidueProof ──────────────────────────────────────────────────────────

   func (s *SQLStore) SaveResidueProof(ctx context.Context, p *LendResidueProof) error {
	  if s == nil || p == nil {
		  return ErrUnavailable
	   }
	   const q = `INSERT INTO lend_proofs
		(proof_id, proof_type, gen_id, order_id, user_id, asset, amount, kind,
		 debit_idem_key, compensate_idem_key, fail_reason, created_at)
		VALUES (?, 'residue', ?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		   p.ProofID, p.GenID, p.OrderID, p.UserID, p.Asset, p.Amount,
		   string(p.Kind), p.DebitIdemKey, p.CompensateIdemKey, p.FailReason, p.CreatedAt,
	  )
	   if err != nil {
		   if isDup(err) {
			   return ErrAlreadyProven
		   }
		  return err
	   }
	   return nil
   }

   func (s *SQLStore) GetResidueProofByGen(ctx context.Context, genID string) (*LendResidueProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  const q = `SELECT proof_id, gen_id, order_id, user_id, asset, amount, kind,
		debit_idem_key, compensate_idem_key, fail_reason, created_at
		FROM lend_proofs WHERE gen_id=? AND proof_type='residue' LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, genID)
	   return scanResidueProof(row)
   }

  func (s *SQLStore) GetResidueProof(ctx context.Context, proofID string) (*LendResidueProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   const q = `SELECT proof_id, gen_id, order_id, user_id, asset, amount, kind,
		debit_idem_key, compensate_idem_key, fail_reason, created_at
		FROM lend_proofs WHERE proof_id=? AND proof_type='residue' LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, proofID)
	  return scanResidueProof(row)
   }

   func scanResidueProof(row *sql.Row) (*LendResidueProof, error) {
	   var p LendResidueProof
	  err := row.Scan(
		   &p.ProofID, &p.GenID, &p.OrderID, &p.UserID, &p.Asset, &p.Amount, &p.Kind,
		   &p.DebitIdemKey, &p.CompensateIdemKey, &p.FailReason, &p.CreatedAt,
	   )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrResidueProofNotFound
		  }
		   return nil, err
	  }
	   return &p, nil
  }

   func (s *SQLStore) ListResidueProofsByOrder(ctx context.Context, orderID string) ([]*LendResidueProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  const q = `SELECT proof_id, gen_id, order_id, user_id, asset, amount, kind,
		debit_idem_key, compensate_idem_key, fail_reason, created_at
		FROM lend_proofs WHERE order_id=? AND proof_type='residue' ORDER BY created_at ASC`
	  rows, err := s.db.QueryContext(ctx, q, orderID)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []*LendResidueProof
	   for rows.Next() {
		   var p LendResidueProof
		   if err := rows.Scan(
			   &p.ProofID, &p.GenID, &p.OrderID, &p.UserID, &p.Asset, &p.Amount, &p.Kind,
			   &p.DebitIdemKey, &p.CompensateIdemKey, &p.FailReason, &p.CreatedAt,
		   ); err != nil {
			   return nil, err
		  }
		  out = append(out, &p)
	   }
	  return out, rows.Err()
   }
