// @FileName: sqlstore.go
// @Summary: clrcreditgate SQL Store 实现（环境，MySQL/PostgreSQL）。
// - 基于 database/sql 口径库；不依赖 ORM。
// - SaveLease INSERT IGNORE 保证幂等；ConsumeLease 使用 UPDATE WHERE state='issued' AND
// fence_epoch=? AND fence_token=? 原子 CAS（rows affected = 0 时返回 ErrLeaseConsumed/ErrLeaseFenceStale）。
// - SaveProof INSERT IGNORE 幂等；credit_ok 防双贷通过唯一索引 uq_clr_proof_item_ok 保证。
// - UpdateBatchDrainStatus UPDATE WHERE status=from 保证 CAS 语义。
// 不变量：
// - 所有时间字段以 UTC 存取。
// - 事务由调用方管理（单语句操作不开事务）。
// @Author: jakson
// @Created: 2021.03.21

package clrcreditgate

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SQLStore 基于 database/sql 的 Store 实现。
type SQLStore struct {
	db *sql.DB
	clock func() time.Time
}

// NewSQLStore 构造 SQLStore。
  func NewSQLStore(db *sql.DB) *SQLStore {
	   return &SQLStore{
		  db: db,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟。
  func (s *SQLStore) WithClock(fn func() time.Time) *SQLStore {
	   if fn != nil {
		   s.clock = fn
	   }
	  return s
   }

  // ── ReleaseCreditLease ───────────────────────────────────────────────────────

   func (s *SQLStore) SaveLease(ctx context.Context, l *ReleaseCreditLease) error {
	  if s == nil || l == nil {
		  return ErrUnavailable
	  }
	   // 检查同 ItemID issued Lease 防重复签发
	   var existCount int
	  _ = s.db.QueryRowContext(ctx,
		   `SELECT COUNT(*) FROM clr_credit_leases WHERE item_id=? AND state='issued'`,
		  l.ItemID,
	  ).Scan(&existCount)
	   if existCount > 0 {
		  return ErrLeaseAlreadyIssued
	  }
	  _, err := s.db.ExecContext(ctx,
		  `INSERT IGNORE INTO clr_credit_leases
		 (lease_id, item_id, batch_id, user_id, asset, amount,
		  issued_at, expires_at, state, actor,
		  fence_epoch, fence_token, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  l.LeaseID, l.ItemID, l.BatchID, l.UserID, l.Asset, l.Amount,
		   l.IssuedAt.UTC(),
		  l.ExpiresAt.UTC(), string(l.State), l.Actor,
		  l.Fence.Epoch, l.Fence.Token, l.CreatedAt.UTC(), l.UpdatedAt.UTC(),
	   )
	  return err
  }

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*ReleaseCreditLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx,
		  `SELECT lease_id, item_id, batch_id, user_id, asset, amount,
		        issued_at, expires_at, state, actor,
		        fence_epoch, fence_token,
		        consumed_at, consumed_by_proof, expired_at, void_at, void_reason,
		        created_at, updated_at
		 FROM clr_credit_leases WHERE lease_id=?`, leaseID)
	   return scanLease(row)
   }

   func (s *SQLStore) GetLeaseByItem(ctx context.Context, itemID string) (*ReleaseCreditLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  // issued 先，再按 issued_at 降序取最新
	  row := s.db.QueryRowContext(ctx,
		  `SELECT lease_id, item_id, batch_id, user_id, asset, amount,
		        issued_at, expires_at, state, actor,
		        fence_epoch, fence_token,
		        consumed_at, consumed_by_proof, expired_at, void_at, void_reason,
		        created_at, updated_at
		 FROM clr_credit_leases WHERE item_id=?
		 ORDER BY CASE WHEN state='issued' THEN 0 ELSE 1 END, issued_at DESC
		 LIMIT 1`, itemID)
	  return scanLease(row)
  }

   func (s *SQLStore) ConsumeLease(ctx context.Context, leaseID string, fence Fence, proofID string, at time.Time) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   // 先读当前状态
	   l, err := s.GetLease(ctx, leaseID)
	  if err != nil {
		  return err
	  }
	   switch l.State {
	  case LeaseConsumed:
		   return ErrLeaseConsumed
	  case LeaseExpired:
		  return ErrLeaseExpired
	   case LeaseVoid:
		  return ErrLeaseVoid
	   }
	  if l.Fence.Epoch != fence.Epoch || l.Fence.Token != fence.Token {
		  return ErrLeaseFenceStale
	   }
	   res, err := s.db.ExecContext(ctx,
		   `UPDATE clr_credit_leases
		 SET state='consumed', consumed_at=?, consumed_by_proof=?, updated_at=?
		 WHERE lease_id=? AND state='issued' AND fence_epoch=? AND fence_token=?`,
		  at.UTC(), proofID, at.UTC(), leaseID, fence.Epoch, fence.Token,
	   )
	  if err != nil {
		   return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  // 并发竞争：再次读取以判断是哪种失败
		  l2, _ := s.GetLease(ctx, leaseID)
		   if l2 != nil && l2.State == LeaseConsumed {
			   return ErrLeaseConsumed
		  }
		   return ErrLeaseFenceStale
	   }
	  return nil
  }

  func (s *SQLStore) MarkLeaseExpired(ctx context.Context, leaseID string, at time.Time) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  res, err := s.db.ExecContext(ctx,
		   `UPDATE clr_credit_leases SET state='expired', expired_at=?, updated_at=?
		 WHERE lease_id=? AND state='issued'`,
		  at.UTC(), at.UTC(), leaseID,
	   )
	  if err != nil {
		  return err
	   }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrInvalidTransition
	  }
	  return nil
  }

  func (s *SQLStore) MarkLeaseVoid(ctx context.Context, leaseID string, reason string, at time.Time) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   res, err := s.db.ExecContext(ctx,
		  `UPDATE clr_credit_leases SET state='void', void_at=?, void_reason=?, updated_at=?
		 WHERE lease_id=? AND state='issued'`,
		   at.UTC(),
		  reason, at.UTC(), leaseID,
	  )
	   if err != nil {
		   return err
	   }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrInvalidTransition
	  }
	   return nil
   }

   func (s *SQLStore) ListLeases(ctx context.Context, f LeaseListFilter) ([]*ReleaseCreditLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  f.Fill()
	   q := `SELECT lease_id, item_id, batch_id, user_id, asset, amount,
	             issued_at, expires_at, state, actor,
	             fence_epoch, fence_token,
	             consumed_at, consumed_by_proof, expired_at, void_at, void_reason,
	             created_at, updated_at
	      FROM clr_credit_leases WHERE 1=1`
	   args := []interface{}{}
	   if f.BatchID != "" {
		   q += " AND batch_id=?"
		  args = append(args, f.BatchID)
	  }
	   if f.ItemID != "" {
		  q += " AND item_id=?"
		  args = append(args, f.ItemID)
	   }
	  if len(f.States) > 0 {
		   q += " AND state IN ("
		  for i, st := range f.States {
			  if i > 0 {
				  q += ","
			   }
			   q += "?"
			  args = append(args, string(st))
		  }
		   q += ")"
	  }
	  if !f.ExpiredBefore.IsZero() {
		  q += " AND expires_at < ?"
		  args = append(args, f.ExpiredBefore.UTC())
	  }
	   if !f.IssuedBefore.IsZero() {
		  q += " AND issued_at < ?"
		   args = append(args,
			  f.IssuedBefore.UTC())
	   }
	  q += " ORDER BY issued_at DESC LIMIT ?"
	   args = append(args, f.Limit)
	   rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]*ReleaseCreditLease, 0)
	  for rows.Next() {
		  l, err := scanLeaseRow(rows)
		   if err != nil {
			   continue
		  }
		  out = append(out, l)
	   }
	   return out, rows.Err()
  }

  func (s *SQLStore) CountUnconsumedLeases(ctx context.Context, batchID string, now time.Time) (int, error) {
	  if s == nil {
		  return 0, ErrUnavailable
	   }
	  var count int
	  err := s.db.QueryRowContext(ctx,
		   `SELECT COUNT(*) FROM clr_credit_leases
		 WHERE batch_id=? AND state='issued' AND expires_at > ?`,
		  batchID, now.UTC(),
	  ).Scan(&count)
	   return count, err
  }

// ── ClearingCreditProof ──────────────────────────────────────────────────────

   func (s *SQLStore) SaveProof(ctx context.Context, p *ClearingCreditProof) error {
	  if s == nil || p == nil {
		  return ErrUnavailable
	   }
	   // credit_ok 防双贷：检查唯一约束（INSERT IGNORE 后验证）
	  if p.Kind == ProofCreditOK {
		   var cnt int
		  _ = s.db.QueryRowContext(ctx,
			  `SELECT COUNT(*) FROM clr_credit_proofs WHERE item_id=? AND kind='credit_ok'`,
			   p.ItemID,
		  ).Scan(&cnt)
		  if cnt > 0 {
			  return ErrAlreadyProven
		   }
	   }
	  _, err := s.db.ExecContext(ctx,
		  `INSERT IGNORE INTO clr_credit_proofs
		 (proof_id, lease_id, item_id, batch_id, user_id, asset, amount,
		  kind, detail, actor, at, epoch_at_proof, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   p.ProofID, p.LeaseID, p.ItemID, p.BatchID, p.UserID, p.Asset, p.Amount,
		   string(p.Kind), p.Detail, p.Actor, p.At.UTC(), p.EpochAtProof, p.CreatedAt.UTC(),
	  )
	   return err
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*ClearingCreditProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx,
		  `SELECT proof_id, lease_id, item_id, batch_id, user_id, asset, amount,
		        kind, detail, actor, at, epoch_at_proof, created_at
		 FROM clr_credit_proofs WHERE proof_id=?`, proofID)
	   return scanProof(row)
   }

   func (s *SQLStore) GetOKProofByItem(ctx context.Context, itemID string) (*ClearingCreditProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx,
		  `SELECT proof_id, lease_id, item_id, batch_id, user_id, asset, amount,
		        kind, detail, actor, at, epoch_at_proof, created_at
		 FROM clr_credit_proofs WHERE item_id=? AND kind='credit_ok' LIMIT 1`, itemID)
	  return scanProof(row)
   }

  func (s *SQLStore) ListProofs(ctx context.Context, f ProofListFilter) ([]*ClearingCreditProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  f.Fill()
	  q := `SELECT proof_id, lease_id, item_id, batch_id, user_id, asset, amount, 
	             kind, detail, actor, at, epoch_at_proof, created_at
	      FROM clr_credit_proofs WHERE 1=1`
	   args := []interface{}{}
	   if f.BatchID != "" {
		   q += " AND batch_id=?"
		   args = append(args, f.BatchID)
	   }
	   if f.ItemID != "" {
		  q += " AND item_id=?"
		  args = append(args, f.ItemID)
	  }
	   if len(f.Kinds) > 0 {
		   q += " AND kind IN ("
		  for i, k := range f.Kinds {
			  if i > 0 {
				  q += ","
			   }
			  q += "?"
			  args = append(args, string(k))
		   }
		  q += ")"
	  }
	  q += " ORDER BY at DESC LIMIT ?"
	  args = append(args, f.Limit)
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]*ClearingCreditProof, 0)
	  for rows.Next() {
		   p, err := scanProofRow(rows)
		   if err != nil {
			  continue
		  }
		  out = append(out, p)
	   }
	   return out, rows.Err()
  }

// ── BatchDrainState ──────────────────────────────────────────────────────────

   func (s *SQLStore) GetOrCreateBatchDrain(ctx context.Context, batchID string, now time.Time) (*BatchDrainState, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx,
		  `INSERT IGNORE INTO clr_batch_drain
		 (batch_id, status, total_items, released_items, applied_items,
		  unconsumed_leases, in_flight_applies, excepted_items,
		  snapshot_at, created_at, updated_at)
		 VALUES (?, 'pending', 0, 0, 0, 0, 0, 0, ?, ?, ?)`,
		  batchID,
		  now.UTC(), now.UTC(), now.UTC(),
	  )
	   if err != nil {
		   return nil, err
	   }
	   return s.GetBatchDrain(ctx, batchID)
   }

  func (s *SQLStore) UpdateBatchDrainStatus(ctx context.Context, batchID string, from, to DrainStatus, patch *BatchDrainState) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  now := s.clock()
	  snapshotAt := now
	  updatedAt := now
	  if patch != nil && !patch.SnapshotAt.IsZero() {
		  snapshotAt = patch.SnapshotAt
	   }
	  if patch != nil && !patch.UpdatedAt.IsZero() {
		  updatedAt = patch.UpdatedAt
	  }
	  unconsumed, inFlight := 0, 0
	  if patch != nil {
		  unconsumed = patch.UnconsumedLeases
		   inFlight = patch.InFlightApplies
	  }
	   res, err := s.db.ExecContext(ctx,
		   `UPDATE clr_batch_drain
		 SET status=?, unconsumed_leases=?, in_flight_applies=?,
		     snapshot_at=?, updated_at=?
		 WHERE batch_id=? AND status=?`,
		  string(to), unconsumed, inFlight, snapshotAt.UTC(), updatedAt.UTC(),
		   batchID, string(from),
	  )
	  if err != nil {
		  return err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	   }
	  return nil
   }

   func (s *SQLStore) GetBatchDrain(ctx context.Context, batchID string) (*BatchDrainState, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx,
		  `SELECT batch_id, status, total_items, released_items, applied_items,  
		        unconsumed_leases, in_flight_applies, excepted_items,
		        drain_started_at, drain_timeout_at, snapshot_at,
		        fence_epoch, fence_token, created_at, updated_at
		 FROM clr_batch_drain WHERE batch_id=?`, batchID)
	  return scanDrain(row)
   }

// ── scan helpers ─────────────────────────────────────────────────────────────

type leaseRowScanner interface {
	Scan(dest ...interface{}) error
}

  func scanLease(row leaseRowScanner) (*ReleaseCreditLease, error) {
	  var l ReleaseCreditLease
	  var consumedAt, expiredAt, voidAt sql.NullTime
	   var consumedByProof, voidReason sql.NullString
	   err := row.Scan(
		   &l.LeaseID, &l.ItemID, &l.BatchID, &l.UserID, &l.Asset, &l.Amount,
		   &l.IssuedAt,
		   &l.ExpiresAt, &l.State, &l.Actor,
		  &l.Fence.Epoch, &l.Fence.Token,
		  &consumedAt, &consumedByProof, &expiredAt, &voidAt, &voidReason,
		   &l.CreatedAt, &l.UpdatedAt,
	  )
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrLeaseNotFound
		  }
		   return nil, err
	  }
	  if consumedAt.Valid {
		   l.ConsumedAt = consumedAt.Time
	  }
	   if consumedByProof.Valid {
		  l.ConsumedByProof = consumedByProof.String
	   }
	  if expiredAt.Valid {
		  l.ExpiredAt = expiredAt.Time
	  }
	  if voidAt.Valid {
		  l.VoidAt = voidAt.Time
	   }
	  if voidReason.Valid {
		  l.VoidReason = voidReason.String
	   }
	  return &l, nil
   }

type leaseRows interface {
	Scan(dest ...interface{}) error
}

   func scanLeaseRow(row leaseRows) (*ReleaseCreditLease, error) {
	  return scanLease(row)
  }

type proofRowScanner interface {
	Scan(dest ...interface{}) error
}

  func scanProof(row proofRowScanner) (*ClearingCreditProof, error) {
	  var p ClearingCreditProof
	  var detail sql.NullString
	   err := row.Scan(
		   &p.ProofID, &p.LeaseID, &p.ItemID, &p.BatchID, &p.UserID, &p.Asset, &p.Amount,
		   &p.Kind, &detail, &p.Actor, &p.At, &p.EpochAtProof, &p.CreatedAt,
	   )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrProofNotFound
		   }
		   return nil, err
	   }
	   if detail.Valid {
		   p.Detail = detail.String
	   }
	   return &p, nil
  }

   func scanProofRow(row proofRowScanner) (*ClearingCreditProof, error) {
	   return scanProof(row)
  }

type drainRowScanner interface {
	Scan(dest ...interface{}) error
}

  func scanDrain(row drainRowScanner) (*BatchDrainState, error) {
	   var d BatchDrainState
	   var drainStartedAt, drainTimeoutAt sql.NullTime
	  err := row.Scan(
		  &d.BatchID,
		   &d.Status, &d.TotalItems, &d.ReleasedItems, &d.AppliedItems,
		   &d.UnconsumedLeases,
		  &d.InFlightApplies, &d.ExceptedItems,
		  &drainStartedAt, &drainTimeoutAt, &d.SnapshotAt,
		   &d.Fence.Epoch, &d.Fence.Token, &d.CreatedAt, &d.UpdatedAt,
	  )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrBatchNotFound
		   }
		  return nil, err
	  }
	  if drainStartedAt.Valid {
		  d.DrainStartedAt = drainStartedAt.Time
	   }
	   if drainTimeoutAt.Valid {
		  d.DrainTimeoutAt = drainTimeoutAt.Time
	  }
	   return &d, nil
  }
