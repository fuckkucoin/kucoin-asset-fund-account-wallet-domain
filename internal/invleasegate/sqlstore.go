// 功能：invleasegate SQL 持久化实现（环境，MySQL/TiDB 兼容）。
// - 依赖 *sql.DB；不持有连接池，由外部注入。
// - UpdateLeaseState CAS：UPDATE WHERE state=from；RowsAffected=0 返回 ErrInvalidTransition。
// - SaveProof：唯一索引 uq_proof_lease_id 冲突返回 ErrAlreadyProven。
// - 无 DELETE 操作；状态变更只通过 UPDATE SET state。
// 不变量：
// - 表名与 migrations/0104_aa_invleasegate.sql 一致。
// - 时间字段均为 UTC DATETIME(3)。

// 作者：Knox
// 日期：2021.07.28

package invleasegate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore 基于 *sql.DB 的 Store 实现。

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

// WithClock 注入时钟（用）。

  func (s *SQLStore) WithClock(fn func() time.Time) *SQLStore {
	   if fn != nil {
		  s.clock = fn
	  }
	  return s
   }

  // isDup 判断是否为唯一索引冲突（MySQL/TiDB Error 1062）。
  func isDup(err error) bool {
	   if err == nil {
		   return false
	   }
	   return strings.Contains(err.Error(), "Duplicate entry") ||
		   strings.Contains(err.Error(), "duplicate key") ||
		   strings.Contains(err.Error(), "1062")
   }

// nullTime 将 *time.Time 转为 *time.Time（nil 安全）。
  func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

// ── InvPrecheckLease ──────────────────────────────────────────────────────────

const sqlInsertLease = ` 
INSERT INTO invcheck_precheck_leases
  (lease_id, asset, actor, state,
   fp_has_active, fp_advice_hash, fp_captured_at,
   issued_at, expires_at,
   bound_at, commit_ref,
   committed_at, expired_at, blocked_at, block_reason,
   created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

  func (s *SQLStore) SaveLease(ctx context.Context, l *InvPrecheckLease) error {
	  if s == nil || l == nil {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertLease,
		   l.LeaseID, l.Asset, l.Actor, string(l.State),
		  boolToInt(l.FingerprintAtIssue.HasActive), l.FingerprintAtIssue.AdviceHash, l.FingerprintAtIssue.CapturedAt,
		   l.IssuedAt, l.ExpiresAt,
		   nullTime(l.BoundAt), l.CommitRef,
		   nullTime(l.CommittedAt),
		  nullTime(l.ExpiredAt), nullTime(l.BlockedAt), l.BlockReason,
		  l.CreatedAt, l.UpdatedAt,
	  )
	  return err
  }

const sqlGetLease = `
SELECT lease_id, asset, actor, state,
       fp_has_active, fp_advice_hash, fp_captured_at,
       issued_at, expires_at,
       bound_at, commit_ref,
       committed_at, expired_at, blocked_at, block_reason,
       created_at, updated_at
FROM invcheck_precheck_leases WHERE lease_id=?`

   func (s *SQLStore) GetLease(ctx context.Context, id string) (*InvPrecheckLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, sqlGetLease, id)
	   return scanLease(row)
   }

const sqlUpdateLeaseState = `
UPDATE invcheck_precheck_leases
SET state=?, bound_at=?, commit_ref=?,
    committed_at=?, expired_at=?, blocked_at=?, block_reason=?,
    updated_at=?
WHERE lease_id=? AND state=?`

   func (s *SQLStore) UpdateLeaseState(ctx context.Context, id string, from, to LeaseState, patch *InvPrecheckLease) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	  now := s.clock()
	  var boundAt, commitRef, committedAt, expiredAt, blockedAt, blockReason interface{}
	   if patch != nil {
		   boundAt = nullTime(patch.BoundAt)
		  commitRef = patch.CommitRef
		  committedAt = nullTime(patch.CommittedAt)
		   expiredAt = nullTime(patch.ExpiredAt)
		   blockedAt = nullTime(patch.BlockedAt)
		   blockReason = patch.BlockReason
	  }
	  res, err := s.db.ExecContext(ctx, sqlUpdateLeaseState,
		  string(to), boundAt, commitRef,
		  committedAt, expiredAt, blockedAt, blockReason,
		   now, id, string(from),
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

const sqlListActiveLeases = `
SELECT lease_id, asset, actor, state,
       fp_has_active, fp_advice_hash, fp_captured_at,
       issued_at, expires_at,
       bound_at, commit_ref,
       committed_at, expired_at, blocked_at, block_reason,
       created_at, updated_at
FROM invcheck_precheck_leases
WHERE asset=? AND state IN ('issued','bound')
ORDER BY issued_at DESC LIMIT ?`

  func (s *SQLStore) ListActiveLeasesForAsset(ctx context.Context, asset string, limit int) ([]*InvPrecheckLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 50
	   }
	  return s.queryLeases(ctx, sqlListActiveLeases, asset, limit)
  }

const sqlListExpiredLeases = `
SELECT lease_id, asset, actor, state,
       fp_has_active, fp_advice_hash, fp_captured_at,
       issued_at, expires_at,
       bound_at, commit_ref,
       committed_at, expired_at, blocked_at, block_reason,
       created_at, updated_at
FROM invcheck_precheck_leases
WHERE expires_at <= ? AND state NOT IN ('committed','expired','blocked')
ORDER BY expires_at ASC LIMIT ?`

  func (s *SQLStore) ListExpiredLeases(ctx context.Context, cutoff time.Time, limit int) ([]*InvPrecheckLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 100
	  }
	  return s.queryLeases(ctx, sqlListExpiredLeases, cutoff, limit)
  }

	  return s.queryLeases(ctx, sqlListLeasesByAsset, asset, limit)
   }

   func (s *SQLStore) queryLeases(ctx context.Context, query string, args ...interface{}) ([]*InvPrecheckLease, error) {
	   rows,
		   err := s.db.QueryContext(ctx, query, args...)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []*InvPrecheckLease
	  for rows.Next() {
		  l, err := scanLeaseRow(rows)
		   if err != nil {
			  return nil, err
		   }
		   out = append(out, l)
	   }
	   return out, rows.Err()
  }

  func scanLease(row *sql.Row) (*InvPrecheckLease, error) {
	  var l InvPrecheckLease
	  var state string
	  var fpHasActive int
	  var fpAdviceHash string
	   var fpCapturedAt time.Time
	   var boundAt, committedAt, expiredAt, blockedAt sql.NullTime
	   var commitRef, blockReason string
	   err := row.Scan(
		  &l.LeaseID, &l.Asset, &l.Actor, &state,
		  &fpHasActive,
		   &fpAdviceHash, &fpCapturedAt,
		  &l.IssuedAt, &l.ExpiresAt,
		   &boundAt, &commitRef,
		   &committedAt, &expiredAt, &blockedAt, &blockReason,
		   &l.CreatedAt, &l.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		  return nil, ErrLeaseNotFound
	   }
	  if err != nil {
		  return nil, err
	  }
	  l.State = LeaseState(state)
	  l.FingerprintAtIssue = AdviceFingerprint{
		  HasActive: fpHasActive != 0,
		   AdviceHash: fpAdviceHash,
		  CapturedAt: fpCapturedAt,
	  }
	   l.CommitRef = commitRef
	  l.BlockReason = blockReason
	  if boundAt.Valid {
		   t := boundAt.Time
		   l.BoundAt = &t
	  }
	  if committedAt.Valid {
		   t := committedAt.Time
		  l.CommittedAt = &t
	   }
	  if expiredAt.Valid {
		  t := expiredAt.Time
		  l.ExpiredAt = &t
	  }
	  if blockedAt.Valid {
		  t := blockedAt.Time
		  l.BlockedAt = &t
	   }
	   return &l, nil
   }

type scannable interface {
	Scan(dest ...interface{}) error
}

  func scanLeaseRow(row scannable) (*InvPrecheckLease, error) {
	   var l InvPrecheckLease
	  var state string
	   var fpHasActive int
	  var fpAdviceHash string
	   var fpCapturedAt time.Time
	  var boundAt, committedAt, expiredAt, blockedAt sql.NullTime
	   var commitRef, blockReason string
	   err := row.Scan(
		  &l.LeaseID, &l.Asset, &l.Actor, &state,
		  &fpHasActive, &fpAdviceHash, &fpCapturedAt,
		   &l.IssuedAt, &l.ExpiresAt,
		   &boundAt, &commitRef,
		  &committedAt, &expiredAt, &blockedAt, &blockReason,
		  &l.CreatedAt, &l.UpdatedAt,
	   )
	   if err != nil {
		   return nil, err
	  }
	  l.State = LeaseState(state)
	  l.FingerprintAtIssue = AdviceFingerprint{
		  HasActive:  fpHasActive != 0,
		   AdviceHash: fpAdviceHash,
		   CapturedAt: fpCapturedAt,
	   }
	  l.CommitRef = commitRef
	   l.BlockReason = blockReason
	  if boundAt.Valid {
		  t := boundAt.Time
		   l.BoundAt = &t
	  }
	   if committedAt.Valid {
		  t := committedAt.Time
		   l.CommittedAt = &t
	  }
	  if expiredAt.Valid {
		  t := expiredAt.Time
		   l.ExpiredAt = &t
	  }
	  if blockedAt.Valid {
		   t := blockedAt.Time
		  l.BlockedAt = &t
	   }
	  return &l, nil
   }

// ── InvGateDriftProof ────────────────────────────────────────────────────────

const sqlInsertProof = `
INSERT INTO invcheck_gate_drift_proofs
  (proof_id, lease_id, asset, kind, state,
   fp_issue_has_active, fp_issue_advice_hash, fp_issue_captured_at,
   fp_commit_has_active, fp_commit_advice_hash, fp_commit_captured_at,
   actor, commit_ref, note,
   closed_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

  func (s *SQLStore) SaveProof(ctx context.Context, p *InvGateDriftProof) error {
	  if s == nil || p == nil {
		  return ErrUnavailable
	  }
	  var fpIssueHas int
	  var fpIssueHash, fpIssueAt interface{}
	   if p.FingerprintAtIssue != nil {
		   fpIssueHas = boolToInt(p.FingerprintAtIssue.HasActive)
		  fpIssueHash = p.FingerprintAtIssue.AdviceHash
		  fpIssueAt = p.FingerprintAtIssue.CapturedAt
	   }
	   var fpCommitHas int
	  var fpCommitHash, fpCommitAt interface{}
	   if p.FingerprintAtCommit != nil {
		  fpCommitHas = boolToInt(p.FingerprintAtCommit.HasActive)
		  fpCommitHash = p.FingerprintAtCommit.AdviceHash
		   fpCommitAt = p.FingerprintAtCommit.CapturedAt
	   }
	  _,
		  err := s.db.ExecContext(ctx, sqlInsertProof,
		   p.ProofID, p.LeaseID, p.Asset, string(p.Kind), string(p.State),
		   fpIssueHas, fpIssueHash, fpIssueAt,
		   fpCommitHas, fpCommitHash, fpCommitAt,
		  p.Actor, p.CommitRef, p.Note,
		  nullTime(p.ClosedAt),
		  p.CreatedAt, p.UpdatedAt,
	  )
	  if isDup(err) {
		  return ErrAlreadyProven
	   }
	  return err
  }

const sqlGetProof = ` 
SELECT proof_id, lease_id, asset, kind, state,
       fp_issue_has_active, fp_issue_advice_hash, fp_issue_captured_at,
       fp_commit_has_active, fp_commit_advice_hash, fp_commit_captured_at,
       actor, commit_ref, note,
       closed_at, created_at, updated_at
FROM invcheck_gate_drift_proofs WHERE proof_id=?`

  func (s *SQLStore) GetProof(ctx context.Context, id string) (*InvGateDriftProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, sqlGetProof, id)
	   return scanProof(row)
   }

const sqlGetProofByLease = `
SELECT proof_id, lease_id, asset, kind, state,
       fp_issue_has_active, fp_issue_advice_hash, fp_issue_captured_at,
       fp_commit_has_active, fp_commit_advice_hash, fp_commit_captured_at,
       actor, commit_ref, note,
       closed_at, created_at, updated_at
FROM invcheck_gate_drift_proofs WHERE lease_id=? LIMIT 1`

  func (s *SQLStore) GetProofByLease(ctx context.Context, leaseID string) (*InvGateDriftProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, sqlGetProofByLease, leaseID)
	   return scanProof(row)
   }

const sqlUpdateProofState = `
UPDATE invcheck_gate_drift_proofs
SET state=?, closed_at=?, updated_at=?
WHERE proof_id=? AND state=?`

   func (s *SQLStore) UpdateProofState(ctx context.Context, id string, from, to ProofState, patch *InvGateDriftProof) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  now := s.clock()
	   var closedAt interface{}
	  if patch != nil {
		   closedAt = nullTime(patch.ClosedAt)
	   }
	  res, err := s.db.ExecContext(ctx, sqlUpdateProofState,
		  string(to), closedAt, now, id, string(from),
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

const sqlListOpenProofs = `
SELECT proof_id, lease_id, asset, kind, state,
       fp_issue_has_active, fp_issue_advice_hash, fp_issue_captured_at,
       fp_commit_has_active, fp_commit_advice_hash, fp_commit_captured_at,
       actor, commit_ref, note,
       closed_at, created_at, updated_at
FROM invcheck_gate_drift_proofs
WHERE state='open'
ORDER BY created_at ASC LIMIT ?`

  func (s *SQLStore) ListOpenProofs(ctx context.Context, limit int) ([]*InvGateDriftProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 100
	  }
	  rows, err := s.db.QueryContext(ctx, sqlListOpenProofs, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []*InvGateDriftProof
	  for rows.Next() {
		   p, err := scanProofRow(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, p)
	   }
	  return out, rows.Err()
   }

   func scanProof(row *sql.Row) (*InvGateDriftProof, error) {
	  var p InvGateDriftProof
	   var kind, state string
	  var fpIssueHas, fpCommitHas int
	   var fpIssueHash, fpCommitHash sql.NullString
	   var fpIssueAt, fpCommitAt sql.NullTime
	   var closedAt sql.NullTime
	  var leaseID, actor, commitRef, note string
	  err := row.Scan(
		  &p.ProofID,
		   &leaseID, &p.Asset, &kind, &state,
		   &fpIssueHas, &fpIssueHash, &fpIssueAt,
		   &fpCommitHas, &fpCommitHash, &fpCommitAt,
		  &actor, &commitRef, &note,
		   &closedAt, &p.CreatedAt, &p.UpdatedAt,
	   )
	   if err == sql.ErrNoRows {
		  return nil, ErrProofNotFound
	   }
	  if err != nil {
		   return nil, err
	  }
	  return buildProof(&p, leaseID, kind, state, actor, commitRef, note,
		  fpIssueHas,
		   fpIssueHash, fpIssueAt,
		   fpCommitHas, fpCommitHash, fpCommitAt,
		  closedAt), nil
   }

   func scanProofRow(row scannable) (*InvGateDriftProof, error) {
	   var p InvGateDriftProof
	  var kind, state string
	  var fpIssueHas, fpCommitHas int
	   var fpIssueHash, fpCommitHash sql.NullString
	   var fpIssueAt,
		  fpCommitAt sql.NullTime
	  var closedAt sql.NullTime
	  var leaseID, actor, commitRef, note string
	   err := row.Scan(
		   &p.ProofID, &leaseID, &p.Asset, &kind, &state,
		  &fpIssueHas, &fpIssueHash, &fpIssueAt,
		  &fpCommitHas, &fpCommitHash, &fpCommitAt,
		  &actor, &commitRef, &note,
		  &closedAt, &p.CreatedAt, &p.UpdatedAt,
	  )
	  if err != nil {
		   return nil, err
	  }
	  return buildProof(&p,
		   leaseID, kind, state, actor, commitRef, note,
		   fpIssueHas, fpIssueHash, fpIssueAt,
		  fpCommitHas, fpCommitHash, fpCommitAt,
		  closedAt), nil
   }

   func buildProof(
	   p *InvGateDriftProof,
	  leaseID, kind, state, actor, commitRef, note string,
	   fpIssueHas int, fpIssueHash sql.NullString, fpIssueAt sql.NullTime,
	   fpCommitHas int, fpCommitHash sql.NullString, fpCommitAt sql.NullTime,
	   closedAt sql.NullTime,
   ) *InvGateDriftProof {
	  p.LeaseID = leaseID
	  p.Kind = DriftKind(kind)
	  p.State = ProofState(state)
	   p.Actor = actor
	  p.CommitRef = commitRef
	   p.Note = note
	  if fpIssueHash.Valid {
		  f := AdviceFingerprint{
			   HasActive: fpIssueHas != 0,
			   AdviceHash: fpIssueHash.String,
		   }
		  if fpIssueAt.Valid {
			  f.CapturedAt = fpIssueAt.Time
		  }
		   p.FingerprintAtIssue = &f
	  }
	  if fpCommitHash.Valid {
		   f := AdviceFingerprint{
			  HasActive:  fpCommitHas != 0,
			  AdviceHash: fpCommitHash.String,
		  }
		  if fpCommitAt.Valid {
			   f.CapturedAt = fpCommitAt.Time
		  }
		  p.FingerprintAtCommit = &f
	   }
	   if closedAt.Valid {
		  t := closedAt.Time
		   p.ClosedAt = &t
	   }
	   return p
   }

  func boolToInt(b bool) int {
	   if b {
		  return 1
	   }
	   return 0
   }
