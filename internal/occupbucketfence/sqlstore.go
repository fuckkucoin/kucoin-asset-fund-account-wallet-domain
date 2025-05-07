// 功能：occupbucketfence SQL 仓储——dep_occup_bucket_fences / break_proofs / half_cases。
// - 兼容 MySQL 占位符；DSN 未注入时不使用本实现。
// - 不写限额租约表。
// 不变量：
// - INSERT 幂等；CAS 用 WHERE state=?。

// 作者：Likeo
// 日期：2025.02.19

package occupbucketfence

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SQLStore 关系库实现。

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 包装 *sql.DB。

  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
  }

   func (s *SQLStore) SaveFence(ctx context.Context, f *BucketFence) error {
	  if s == nil || s.db == nil || f == nil {
		   return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO dep_occup_bucket_fences (
  fence_id, lot_id, user_id, asset, bucket_fingerprint, generation, ttl_seconds, state,
  bound_hold_id, consumed_by, issued_at, bound_at, consumed_at, expires_at, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE fence_id=fence_id`,
		  f.FenceID, f.LotID, f.UserID, f.Asset, f.BucketFingerprint, f.Generation, f.TTLSeconds, string(f.State),
		   f.BoundHoldID,
		  f.ConsumedBy, f.IssuedAt, nullTime(f.BoundAt), nullTime(f.ConsumedAt), f.ExpiresAt, f.CreatedAt)
	   return err
   }

  func (s *SQLStore) GetFence(ctx context.Context, fenceID string) (*BucketFence, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `
SELECT fence_id, lot_id, user_id, asset, bucket_fingerprint, generation, ttl_seconds, state,
       bound_hold_id, consumed_by, issued_at, bound_at, consumed_at, expires_at, created_at
  FROM dep_occup_bucket_fences WHERE fence_id=?`, fenceID)
	   return scanFence(row)
  }

  func (s *SQLStore) GetActiveFenceByLot(ctx context.Context, lotID string) (*BucketFence, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, `
SELECT fence_id, lot_id, user_id, asset, bucket_fingerprint, generation, ttl_seconds, state,
       bound_hold_id, consumed_by, issued_at, bound_at, consumed_at, expires_at, created_at
  FROM dep_occup_bucket_fences
 WHERE lot_id=? AND state IN ('issued','bound')
 ORDER BY issued_at DESC LIMIT 1`, lotID)
	  return scanFence(row)
   }

  func (s *SQLStore) UpdateFence(ctx context.Context, f *BucketFence) error {
	   if s == nil || s.db == nil || f == nil {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, `
UPDATE dep_occup_bucket_fences SET
  bucket_fingerprint=?, generation=?, state=?, bound_hold_id=?, consumed_by=?,
  bound_at=?, consumed_at=?, expires_at=?
 WHERE fence_id=?`,
		   f.BucketFingerprint, f.Generation, string(f.State), f.BoundHoldID, f.ConsumedBy,
		  nullTime(f.BoundAt), nullTime(f.ConsumedAt), f.ExpiresAt, f.FenceID)
	  return err
  }

  func (s *SQLStore) CASFenceState(ctx context.Context, fenceID string, from, to FenceState, patch *BucketFence) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	  fp, gen, hold, by := "", int64(0), "", ""
	  var boundAt, consAt interface{}
	   if patch != nil {
		   fp, gen, hold, by = patch.BucketFingerprint, patch.Generation, patch.BoundHoldID, patch.ConsumedBy
		   boundAt,
			  consAt = nullTime(patch.BoundAt), nullTime(patch.ConsumedAt)
	   }
	  res, err := s.db.ExecContext(ctx, `
UPDATE dep_occup_bucket_fences SET
  state=?,
  bucket_fingerprint=CASE WHEN ?<>'' THEN ? ELSE bucket_fingerprint END,
  generation=CASE WHEN ?>0 THEN ? ELSE generation END,
  bound_hold_id=CASE WHEN ?<>'' THEN ? ELSE bound_hold_id END,
  consumed_by=CASE WHEN ?<>'' THEN ? ELSE consumed_by END,
  bound_at=COALESCE(?, bound_at),
  consumed_at=COALESCE(?, consumed_at)
 WHERE fence_id=? AND state=?`,
		  string(to), fp, fp, gen, gen, hold, hold, by, by, boundAt, consAt, fenceID, string(from))
	  if err != nil {
		   return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	  }
	   return nil
  }

  func (s *SQLStore) ListExpiredBound(ctx context.Context, now time.Time, limit int) ([]*BucketFence, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 200
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT fence_id, lot_id, user_id, asset, bucket_fingerprint, generation, ttl_seconds, state,
       bound_hold_id, consumed_by, issued_at, bound_at, consumed_at, expires_at, created_at
  FROM dep_occup_bucket_fences
 WHERE state IN ('issued','bound') AND expires_at < ?
 LIMIT ?`, now, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := make([]*BucketFence, 0)
	   for rows.Next() {
		  f, err := scanFenceRows(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, f)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) ListFencesByLot(ctx context.Context, lotID string) ([]*BucketFence, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  rows, err := s.db.QueryContext(ctx, `
SELECT fence_id, lot_id, user_id, asset, bucket_fingerprint, generation, ttl_seconds, state,
       bound_hold_id, consumed_by, issued_at, bound_at, consumed_at, expires_at, created_at
  FROM dep_occup_bucket_fences WHERE lot_id=?`, lotID)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  out := make([]*BucketFence, 0)
	  for rows.Next() {
		  f, err := scanFenceRows(rows)
		   if err != nil {
			  return nil, err
		  }
		   out = append(out, f)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) SaveProof(ctx context.Context, p *OccupancyBreakProof) error {
	  if s == nil || s.db == nil || p == nil {
		   return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO dep_occup_break_proofs (
  proof_id, lot_id, fence_id, state, kind, inflight, available, void_amt, sum_amt, total_amt, delta_amt,
  conflict_op, winner_op, reason, digest, opened_at, proven_at, closed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ProofID, p.LotID, p.FenceID, string(p.State), p.Kind, p.Inflight, p.Available, p.Void, p.Sum, p.Total, p.Delta,
		  p.ConflictOp, p.WinnerOp, p.Reason, p.Digest, p.OpenedAt, nullTime(p.ProvenAt), nullTime(p.ClosedAt))
	   if err != nil {
		  return err
	  }
	  return nil
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*OccupancyBreakProof, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `
SELECT proof_id, lot_id, fence_id, state, kind, inflight, available, void_amt, sum_amt, total_amt, delta_amt,
       conflict_op, winner_op, reason, digest, opened_at, proven_at, closed_at
  FROM dep_occup_break_proofs WHERE proof_id=?`, proofID)
	   return scanProof(row)
   }

   func (s *SQLStore) GetOpenProofByLot(ctx context.Context, lotID string) (*OccupancyBreakProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `
SELECT proof_id, lot_id, fence_id, state, kind, inflight, available, void_amt, sum_amt, total_amt, delta_amt,
       conflict_op, winner_op, reason, digest, opened_at, proven_at, closed_at
  FROM dep_occup_break_proofs WHERE lot_id=? AND state IN ('open','proven') ORDER BY opened_at DESC LIMIT 1`, lotID)
	   return scanProof(row)
  }

   func (s *SQLStore) ListProofsByLot(ctx context.Context, lotID string) ([]*OccupancyBreakProof, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT proof_id, lot_id, fence_id, state, kind, inflight, available, void_amt, sum_amt, total_amt, delta_amt,
       conflict_op, winner_op, reason, digest, opened_at, proven_at, closed_at
  FROM dep_occup_break_proofs WHERE lot_id=?`, lotID)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  out := make([]*OccupancyBreakProof,
		  0)
	   for rows.Next() {
		   p, err := scanProofRows(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, p)
	   }
	   return out, rows.Err()
   }

  func (s *SQLStore) UpdateProof(ctx context.Context, p *OccupancyBreakProof) error {
	  if s == nil || s.db == nil || p == nil {
		   return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
UPDATE dep_occup_break_proofs SET state=?, digest=?, proven_at=?, closed_at=? WHERE proof_id=?`,
		  string(p.State), p.Digest, nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.ProofID)
	   return err
  }

  func (s *SQLStore) SaveHalf(ctx context.Context, h *HalfBucketCase) error {
	   if s == nil || s.db == nil || h == nil {
		   return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO dep_occup_half_cases (
  case_id, lot_id, hold_id, fence_id, state, amount, reason, opened_at, resumed_at
) VALUES (?,?,?,?,?,?,?,?,?)`,
		  h.CaseID, h.LotID, h.HoldID, h.FenceID, string(h.State), h.Amount, h.Reason, h.OpenedAt, nullTime(h.ResumedAt))
	  return err
   }

  func (s *SQLStore) GetHalf(ctx context.Context, caseID string) (*HalfBucketCase, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT case_id, lot_id, hold_id, fence_id, state, amount, reason, opened_at, resumed_at
  FROM dep_occup_half_cases WHERE case_id=?`, caseID)
	   return scanHalf(row)
  }

  func (s *SQLStore) GetOpenHalfByLot(ctx context.Context, lotID string) (*HalfBucketCase, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT case_id, lot_id, hold_id, fence_id, state, amount, reason, opened_at, resumed_at
  FROM dep_occup_half_cases WHERE lot_id=? AND state='open' LIMIT 1`, lotID)
	   return scanHalf(row)
   }

  func (s *SQLStore) UpdateHalf(ctx context.Context, h *HalfBucketCase) error {
	  if s == nil || s.db == nil || h == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
UPDATE dep_occup_half_cases SET state=?, resumed_at=? WHERE case_id=?`,
		   string(h.State), nullTime(h.ResumedAt), h.CaseID)
	  return err
   }

   func (s *SQLStore) ListOpenHalves(ctx context.Context, limit int) ([]*HalfBucketCase, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 200
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT case_id, lot_id, hold_id, fence_id, state, amount, reason, opened_at, resumed_at
  FROM dep_occup_half_cases WHERE state='open' LIMIT ?`, limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := make([]*HalfBucketCase, 0)
	  for rows.Next() {
		   h, err := scanHalfRows(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, h)
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) TryLockLotOp(ctx context.Context, lotID, op string) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  res, err := s.db.ExecContext(ctx, `
INSERT INTO dep_occup_lot_mutex (lot_id, op_name, locked_at) VALUES (?,?,?)`,
		  lotID, op, time.Now().UTC())
	   if err != nil {
		  return ErrReclassMutex
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrReclassMutex
	   }
	  return nil
  }

   func (s *SQLStore) UnlockLotOp(ctx context.Context, lotID string) {
	   if s == nil || s.db == nil {
		  return
	   }
	   _, _ = s.db.ExecContext(ctx, `DELETE FROM dep_occup_lot_mutex WHERE lot_id=?`, lotID)
  }

type rowIface interface {
	Scan(dest ...any) error
}

   func scanFence(row rowIface) (*BucketFence, error) {
	   var f BucketFence
	   var boundAt, consAt sql.NullTime
	   var state string
	  err := row.Scan(&f.FenceID, &f.LotID, &f.UserID, &f.Asset, &f.BucketFingerprint, &f.Generation, &f.TTLSeconds, &state,
		  &f.BoundHoldID, &f.ConsumedBy, &f.IssuedAt, &boundAt, &consAt, &f.ExpiresAt, &f.CreatedAt)
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrFenceNotFound
		  }
		   return nil, err
	  }
	  f.State = FenceState(state)
	   if boundAt.Valid {
		  t := boundAt.Time.UTC()
		   f.BoundAt = &t
	  }
	   if consAt.Valid {
		  t := consAt.Time.UTC()
		   f.ConsumedAt = &t
	   }
	  return &f, nil
  }

  func scanFenceRows(rows *sql.Rows) (*BucketFence, error) {
	  return scanFence(rows)
   }

  func scanProof(row rowIface) (*OccupancyBreakProof, error) {
	  var p OccupancyBreakProof
	  var state string
	  var proven, closed sql.NullTime
	  err := row.Scan(&p.ProofID, &p.LotID, &p.FenceID, &state, &p.Kind, &p.Inflight, &p.Available, &p.Void, &p.Sum, &p.Total, &p.Delta,
		   &p.ConflictOp, &p.WinnerOp, &p.Reason, &p.Digest, &p.OpenedAt, &proven, &closed)
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrProofNotFound
		  }
		  return nil, err
	  }
	  p.State = BreakState(state)
	   if proven.Valid {
		   t := proven.Time.UTC()
		  p.ProvenAt = &t
	   }
	   if closed.Valid {
		  t := closed.Time.UTC()
		   p.ClosedAt = &t
	  }
	  return &p, nil
   }

   func scanProofRows(rows *sql.Rows) (*OccupancyBreakProof, error) {
	   return scanProof(rows)
   }

  func scanHalf(row rowIface) (*HalfBucketCase, error) {
	   var h HalfBucketCase
	   var state string
	  var resumed sql.NullTime
	  err := row.Scan(&h.CaseID, &h.LotID, &h.HoldID, &h.FenceID, &state, &h.Amount, &h.Reason, &h.OpenedAt, &resumed)
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrHalfNotFound
		  }
		  return nil, err
	  }
	   h.State = HalfState(state)
	   if resumed.Valid {
		   t := resumed.Time.UTC()
		  h.ResumedAt = &t
	  }
	   return &h, nil
  }

  func scanHalfRows(rows *sql.Rows) (*HalfBucketCase, error) {
	  return scanHalf(rows)
  }

   func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
