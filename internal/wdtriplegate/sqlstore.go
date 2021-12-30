// - 读写 wd_triple_leases、wd_preflight_binds、wd_triple_drift_cases 三张表。
// - AppendBind / AppendDriftCase 使用 INSERT IGNORE 保证 append-only 幂等。
// - UpdateLeaseStateCAS 通过 UPDATE … WHERE state=from 实现乐观锁。
// - 不依赖 ORM；使用 database/sql 口径接口。
// 不变量：
// - 同 movement_id 同 state IN ('issued','bound') 最多一条（DB 层唯一索引辅助）。
// - wd_preflight_binds.bind_id 主键唯一；movement_id 唯一索引。

// Author: Draven
// Date: 2021.12.29

package wdtriplegate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore Store 接口的 SQL 实现（MySQL）。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 构造 SQL 存储。
  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
   }

// ── TripleLease ──────────────────────────────────────────────────────────────

  func (s *SQLStore) SaveLease(lease TripleLease) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  ctx := context.Background()
	   // 先检查同 movement_id 是否有活跃租约
	   var cnt int
	   err := s.db.QueryRowContext(ctx,
		  `SELECT COUNT(*) FROM wd_triple_leases
		  WHERE movement_id = ? AND state IN ('issued','bound')`,
		  lease.MovementID).Scan(&cnt)
	  if err != nil {
		  return err
	  }
	  if cnt > 0 {
		   return ErrBindConflict
	   }
	   _, err = s.db.ExecContext(ctx,
		  `INSERT INTO wd_triple_leases
		 (triple_lease_id, movement_id, user_id, chain, dest,
		  ticket_clear, kyt_pin, list_gen, policy_ver,
		  state, issued_at, expires_at, issued_by, created_at, updated_at)
		 VALUES (?,?,?,?,?, ?,?,?,?, ?,?,?, ?,?,?)`,
		  lease.TripleLeaseID,
		  lease.MovementID, lease.UserID, lease.Chain, lease.Dest,
		  boolToInt(lease.TicketClear), lease.KYTPin, lease.ListGen, lease.PolicyVer,
		  string(lease.State), lease.IssuedAt, lease.ExpiresAt, lease.IssuedBy,
		   lease.CreatedAt, lease.UpdatedAt,
	   )
	  return err
   }

   func (s *SQLStore) GetLease(leaseID string) (*TripleLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	  ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		  `SELECT triple_lease_id, movement_id, user_id, chain, dest,
		        ticket_clear, kyt_pin, list_gen, policy_ver,
		        state, issued_at, expires_at, bound_at, consumed_at,
		        expired_at, blocked_at, drift_case_id, block_reason,
		        issued_by, created_at, updated_at
		   FROM wd_triple_leases WHERE triple_lease_id = ?`, leaseID)
	  return scanLease(row)
  }

   func (s *SQLStore) GetActiveLeaseByMovement(movementID string) (*TripleLease, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  ctx := context.Background()
	  row := s.db.QueryRowContext(ctx,
		   `SELECT triple_lease_id, movement_id, user_id, chain, dest,
		        ticket_clear, kyt_pin, list_gen, policy_ver,
		        state, issued_at, expires_at, bound_at, consumed_at,
		        expired_at, blocked_at, drift_case_id, block_reason,
		        issued_by, created_at, updated_at
		   FROM wd_triple_leases
		  WHERE movement_id = ? AND state IN ('issued','bound')
		  LIMIT 1`, movementID)
	   return scanLease(row)
   }

  func (s *SQLStore) UpdateLeaseStateCAS(leaseID string, from, to LeaseState, now time.Time, extra *LeaseExtra) (*TripleLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   if err := LeaseTransition(from, to); err != nil {
		   return nil, ErrInvalidTransition
	  }
	   ctx := context.Background()
	  var driftCaseID, blockReason sql.NullString
	   if extra != nil {
		  if extra.DriftCaseID != "" {
			  driftCaseID = sql.NullString{String: extra.DriftCaseID,
				   Valid: true}
		  }
		   if extra.BlockReason != "" {
			   blockReason = sql.NullString{String: extra.BlockReason, Valid: true}
		  }
	   }
	   var tsField string
	  switch to {
	  case LeaseBound:
		  tsField = "bound_at"
	   case LeaseConsumed:
		  tsField = "consumed_at"
	  case LeaseExpired:
		  tsField = "expired_at"
	   case LeaseBlocked:
		   tsField = "blocked_at"
	   }
	   query := `UPDATE wd_triple_leases SET state=?, ` + tsField + `=?,
	           drift_case_id=COALESCE(?, drift_case_id),
	           block_reason=COALESCE(?, block_reason),
	           updated_at=?
	           WHERE triple_lease_id=? AND state=?`
	  res, err := s.db.ExecContext(ctx, query,
		  string(to), now,
		   driftCaseID, blockReason,
		  now, leaseID, string(from),
	   )
	   if err != nil {
		  return nil, err
	   }
	  n, err := res.RowsAffected()
	   if err != nil {
		   return nil, err
	   }
	  if n == 0 {
		  return nil, ErrInvalidTransition
	  }
	   return s.GetLease(leaseID)
  }

   func (s *SQLStore) ListExpiredLeases(before time.Time, limit int) ([]*TripleLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   ctx := context.Background()
	  if limit <= 0 {
		  limit = 100
	   }
	  rows, err := s.db.QueryContext(ctx,
		   `SELECT triple_lease_id, movement_id, user_id, chain, dest,
		        ticket_clear, kyt_pin, list_gen, policy_ver,
		        state, issued_at, expires_at, bound_at, consumed_at,
		        expired_at, blocked_at, drift_case_id, block_reason,
		        issued_by, created_at, updated_at
		   FROM wd_triple_leases
		  WHERE state IN ('issued','bound') AND expires_at < ?
		  LIMIT ?`, before, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  return scanLeases(rows)
   }

   func (s *SQLStore) ListLeasesByMovement(movementID string) ([]*TripleLease, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  ctx := context.Background()
	  rows, err := s.db.QueryContext(ctx,
		  `SELECT triple_lease_id, movement_id, user_id, chain, dest, 
		        ticket_clear, kyt_pin, list_gen, policy_ver,
		        state, issued_at, expires_at, bound_at, consumed_at,
		        expired_at, blocked_at, drift_case_id, block_reason,
		        issued_by, created_at, updated_at
		   FROM wd_triple_leases WHERE movement_id = ?
		   ORDER BY created_at DESC`, movementID)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  return scanLeases(rows)
  }

// ── PreflightBind ────────────────────────────────────────────────────────────

  func (s *SQLStore) AppendBind(bind PreflightBind) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	   ctx := context.Background()
	  _, err := s.db.ExecContext(ctx,
		   `INSERT IGNORE INTO wd_preflight_binds
		 (bind_id, movement_id, triple_lease_id,
		  list_gen_at_bind, policy_ver_at_bind,
		  broadcast_allowed, refuse_reason, drift_detected,
		  bound_at, created_at)
		 VALUES (?,?,?, ?,?, ?,?,?, ?,?)`,
		   bind.BindID, bind.MovementID, bind.TripleLeaseID,
		  bind.ListGenAtBind,
		   bind.PolicyVerAtBind,
		  boolToInt(bind.BroadcastAllowed), nullStr(bind.RefuseReason), boolToInt(bind.DriftDetected),
		  bind.BoundAt, bind.CreatedAt,
	  )
	  return err
   }

   func (s *SQLStore) GetBindByMovement(movementID string) (*PreflightBind, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		  `SELECT bind_id, movement_id, triple_lease_id,
		        list_gen_at_bind, policy_ver_at_bind,
		        broadcast_allowed, refuse_reason, drift_detected,
		        bound_at, created_at
		   FROM wd_preflight_binds WHERE movement_id = ?`, movementID)
	  var b PreflightBind
	  var refuseReason sql.NullString
	   var broadcastAllowed, driftDetected int
	  err := row.Scan(
		  &b.BindID, &b.MovementID, &b.TripleLeaseID,
		   &b.ListGenAtBind, &b.PolicyVerAtBind,
		  &broadcastAllowed, &refuseReason, &driftDetected,
		   &b.BoundAt, &b.CreatedAt,
	   )
	   if err == sql.ErrNoRows {
		  return nil, ErrBindNotFound
	   }
	  if err != nil {
		  return nil, err
	  }
	   b.BroadcastAllowed = broadcastAllowed != 0
	  b.DriftDetected = driftDetected != 0
	   if refuseReason.Valid {
		   b.RefuseReason = refuseReason.String
	   }
	  return &b, nil
  }

// ── TripleDriftCase ──────────────────────────────────────────────────────────

  func (s *SQLStore) AppendDriftCase(dc TripleDriftCase) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	   ctx := context.Background()
	   _, err := s.db.ExecContext(ctx,
		  `INSERT IGNORE INTO wd_triple_drift_cases
		 (drift_case_id, movement_id, triple_lease_id, drift_kind,
		  lease_list_gen, current_list_gen, lease_policy_ver, current_policy_ver,
		  lease_kyt_pin, current_kyt_pin,
		  state, proof_checksum, confirmed_by, co_confirmed_by, close_note,
		  detected_at, proven_at, closed_at, created_at, updated_at)
		 VALUES (?,?,?,?, ?,?,?,?, ?,?, ?,?,?,?,?, ?,?,?,?,?)`,
		   dc.DriftCaseID, dc.MovementID, dc.TripleLeaseID, string(dc.DriftKind),
		   dc.LeaseListGen, dc.CurrentListGen, dc.LeasePolicyVer, dc.CurrentPolicyVer,
		   dc.LeaseKYTPin, dc.CurrentKYTPin,
		  string(dc.State), nullStr(dc.ProofChecksum),
		   nullStr(dc.ConfirmedBy),
		  nullStr(dc.CoConfirmedBy), nullStr(dc.CloseNote),
		  dc.DetectedAt, nullTime(dc.ProvenAt), nullTime(dc.ClosedAt),
		  dc.CreatedAt, dc.UpdatedAt,
	   )
	  return err
  }

  func (s *SQLStore) GetDriftCase(driftCaseID string) (*TripleDriftCase, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		   `SELECT drift_case_id, movement_id, triple_lease_id, drift_kind,
		        lease_list_gen, current_list_gen, lease_policy_ver, current_policy_ver,
		        lease_kyt_pin, current_kyt_pin,
		        state, proof_checksum, confirmed_by, co_confirmed_by, close_note,
		        detected_at, proven_at, closed_at, created_at, updated_at
		   FROM wd_triple_drift_cases WHERE drift_case_id = ?`, driftCaseID)
	  return scanDriftCase(row)
   }

  func (s *SQLStore) ListOpenDriftCases(limit int) ([]*TripleDriftCase, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  ctx := context.Background()
	  if limit <= 0 {
		  limit = 100
	   }
	  rows, err := s.db.QueryContext(ctx,
		  `SELECT drift_case_id, movement_id, triple_lease_id, drift_kind,
		        lease_list_gen, current_list_gen, lease_policy_ver, current_policy_ver,
		        lease_kyt_pin, current_kyt_pin,
		        state, proof_checksum, confirmed_by, co_confirmed_by, close_note,
		        detected_at, proven_at, closed_at, created_at, updated_at
		   FROM wd_triple_drift_cases WHERE state = 'open' LIMIT ?`, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanDriftCases(rows)
   }

   func (s *SQLStore) ListDriftCasesByMovement(movementID string) ([]*TripleDriftCase, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  ctx := context.Background()
	  rows, err := s.db.QueryContext(ctx,
		  `SELECT drift_case_id, movement_id, triple_lease_id, drift_kind,
		        lease_list_gen, current_list_gen, lease_policy_ver, current_policy_ver,
		        lease_kyt_pin, current_kyt_pin,
		        state, proof_checksum, confirmed_by, co_confirmed_by, close_note,
		        detected_at, proven_at, closed_at, created_at, updated_at
		   FROM wd_triple_drift_cases WHERE movement_id = ?
		   ORDER BY detected_at DESC`, movementID)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanDriftCases(rows)
  }

  func (s *SQLStore) UpdateDriftCaseState(driftCaseID string, from, to DriftCaseState, now time.Time, extra *DriftCaseExtra) (*TripleDriftCase, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   if err := DriftCaseTransition(from, to); err != nil {
		   return nil, ErrInvalidTransition
	  }
	  ctx := context.Background()
	  var proofChecksum, confirmedBy, coConfirmedBy, closeNote sql.NullString
	  if extra != nil {
		   proofChecksum = sql.NullString{String: extra.ProofChecksum, Valid: extra.ProofChecksum != ""}
		  confirmedBy = sql.NullString{String: extra.ConfirmedBy, Valid: extra.ConfirmedBy != ""}
		  coConfirmedBy = sql.NullString{String: extra.CoConfirmedBy, Valid: extra.CoConfirmedBy != ""}
		   closeNote = sql.NullString{String: extra.CloseNote, Valid: extra.CloseNote != ""}
	   }
	   var tsField string
	   switch to {
	  case DriftProven:
		  tsField = "proven_at"
	   case DriftClosed:
		  tsField = "closed_at"
	   }
	   query := `UPDATE wd_triple_drift_cases SET state=?, ` + tsField + `=?,
	           proof_checksum=COALESCE(?, proof_checksum),
	           confirmed_by=COALESCE(?, confirmed_by),
	           co_confirmed_by=COALESCE(?, co_confirmed_by),
	           close_note=COALESCE(?, close_note),
	           updated_at=?
	           WHERE drift_case_id=? AND state=?`
	  res, err := s.db.ExecContext(ctx, query,
		   string(to), now,
		   proofChecksum, confirmedBy, coConfirmedBy, closeNote,
		  now, driftCaseID, string(from),
	  )
	   if err != nil {
		   return nil, err
	   }
	   n, err := res.RowsAffected()
	   if err != nil {
		  return nil, err
	  }
	  if n == 0 {
		   return nil, ErrInvalidTransition
	   }
	  return s.GetDriftCase(driftCaseID)
  }

// ── 扫描辅助 ─────────────────────────────────────────────────────────────────

type leaseScanner interface {
	Scan(dest ...any) error
}

  func scanLease(row leaseScanner) (*TripleLease, error) {
	   var l TripleLease
	   var ticketClear int
	  var boundAt, consumedAt, expiredAt, blockedAt sql.NullTime
	  var driftCaseID,
		   blockReason sql.NullString
	  err := row.Scan(
		  &l.TripleLeaseID, &l.MovementID, &l.UserID, &l.Chain, &l.Dest,
		   &ticketClear,
		  &l.KYTPin, &l.ListGen, &l.PolicyVer,
		   &l.State, &l.IssuedAt, &l.ExpiresAt,
		  &boundAt, &consumedAt, &expiredAt, &blockedAt,
		  &driftCaseID, &blockReason,
		   &l.IssuedBy,
		  &l.CreatedAt, &l.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		  return nil, ErrLeaseNotFound
	  }
	   if err != nil {
		   return nil, err
	  }
	  l.TicketClear = ticketClear != 0
	   if boundAt.Valid {
		  l.BoundAt = boundAt.Time
	   }
	  if consumedAt.Valid {
		   l.ConsumedAt = consumedAt.Time
	   }
	   if expiredAt.Valid {
		   l.ExpiredAt = expiredAt.Time
	   }
	  if blockedAt.Valid {
		  l.BlockedAt = blockedAt.Time
	  }
	   if driftCaseID.Valid {
		  l.DriftCaseID = driftCaseID.String
	   }
	   if blockReason.Valid {
		   l.BlockReason = blockReason.String
	   }
	   return &l, nil
   }

  func scanLeases(rows *sql.Rows) ([]*TripleLease, error) {
	  var out []*TripleLease
	   for rows.Next() {
		  var l TripleLease
		  var ticketClear int
		   var boundAt, consumedAt, expiredAt, blockedAt sql.NullTime
		   var driftCaseID, blockReason sql.NullString
		   err := rows.Scan(
			  &l.TripleLeaseID, &l.MovementID, &l.UserID, &l.Chain, &l.Dest,
			  &ticketClear,
			   &l.KYTPin, &l.ListGen, &l.PolicyVer,
			   &l.State, &l.IssuedAt, &l.ExpiresAt,
			  &boundAt, &consumedAt, &expiredAt, &blockedAt,
			  &driftCaseID, &blockReason,
			  &l.IssuedBy,
			  &l.CreatedAt, &l.UpdatedAt,
		  )
		   if err != nil {
			  return nil, err
		  }
		   l.TicketClear = ticketClear != 0
		   if boundAt.Valid {
			   l.BoundAt = boundAt.Time
		   }
		   if consumedAt.Valid {
			   l.ConsumedAt = consumedAt.Time
		  }
		  if expiredAt.Valid {
			   l.ExpiredAt = expiredAt.Time
		   }
		   if blockedAt.Valid {
			   l.BlockedAt = blockedAt.Time
		  }
		   if driftCaseID.Valid {
			  l.DriftCaseID = driftCaseID.String
		  }
		   if blockReason.Valid {
			  l.BlockReason = blockReason.String
		   }
		   cp := l
		   out = append(out, &cp)
	   }
	  return out, rows.Err()
   }

type driftCaseScanner interface {
	Scan(dest ...any) error
}

   func scanDriftCase(row driftCaseScanner) (*TripleDriftCase, error) {
	   var dc TripleDriftCase
	  var provenAt, closedAt sql.NullTime
	  var proofChecksum, confirmedBy, coConfirmedBy, closeNote sql.NullString
	   var driftKind string
	  err := row.Scan(
		  &dc.DriftCaseID, &dc.MovementID, &dc.TripleLeaseID, &driftKind,
		  &dc.LeaseListGen, &dc.CurrentListGen, &dc.LeasePolicyVer, &dc.CurrentPolicyVer,
		   &dc.LeaseKYTPin, &dc.CurrentKYTPin,
		   &dc.State, &proofChecksum, &confirmedBy, &coConfirmedBy, &closeNote,
		  &dc.DetectedAt, &provenAt, &closedAt, &dc.CreatedAt, &dc.UpdatedAt,
	   )
	  if err == sql.ErrNoRows {
		   return nil,
			  ErrDriftCaseNotFound
	   }
	   if err != nil {
		   return nil, err
	   }
	  dc.DriftKind = DriftKind(driftKind)
	  if provenAt.Valid {
		  dc.ProvenAt = provenAt.Time
	   }
	   if closedAt.Valid {
		  dc.ClosedAt = closedAt.Time
	  }
	  if proofChecksum.Valid {
		  dc.ProofChecksum = proofChecksum.String
	   }
	  if confirmedBy.Valid {
		  dc.ConfirmedBy = confirmedBy.String
	   }
	  if coConfirmedBy.Valid {
		   dc.CoConfirmedBy = coConfirmedBy.String
	   }
	   if closeNote.Valid {
		  dc.CloseNote = closeNote.String
	   }
	   return &dc, nil
   }

  func scanDriftCases(rows *sql.Rows) ([]*TripleDriftCase, error) {
	  var out []*TripleDriftCase
	  for rows.Next() {
		   var dc TripleDriftCase
		  var provenAt, closedAt sql.NullTime
		   var proofChecksum, confirmedBy, coConfirmedBy, closeNote sql.NullString
		   var driftKind string
		   err := rows.Scan(
			  &dc.DriftCaseID, &dc.MovementID, &dc.TripleLeaseID, &driftKind,
			  &dc.LeaseListGen, &dc.CurrentListGen, &dc.LeasePolicyVer, &dc.CurrentPolicyVer,
			  &dc.LeaseKYTPin, &dc.CurrentKYTPin,
			   &dc.State, &proofChecksum, &confirmedBy, &coConfirmedBy, &closeNote,
			   &dc.DetectedAt, &provenAt, &closedAt, &dc.CreatedAt, &dc.UpdatedAt,
		   )
		  if err != nil {
			   return nil, err
		   }
		  dc.DriftKind = DriftKind(driftKind)
		   if provenAt.Valid {
			   dc.ProvenAt = provenAt.Time
		   }
		   if closedAt.Valid {
			  dc.ClosedAt = closedAt.Time
		  }
		  if proofChecksum.Valid {
			  dc.ProofChecksum = proofChecksum.String
		   }
		  if confirmedBy.Valid {
			   dc.ConfirmedBy = confirmedBy.String
		  }
		  if coConfirmedBy.Valid {
			   dc.CoConfirmedBy = coConfirmedBy.String
		   }
		   if closeNote.Valid {
			   dc.CloseNote = closeNote.String
		  }
		  cp := dc
		   out = append(out, &cp)
	   }
	  return out, rows.Err()
   }

// ── 辅助函数 ──────────────────────────────────────────────────────────────────

  func boolToInt(b bool) int {
	   if b {
		   return 1
	   }
	  return 0
   }

   func nullStr(s string) sql.NullString {
	   return sql.NullString{String: s, Valid: strings.TrimSpace(s) != ""}
  }

   func nullTime(t time.Time) sql.NullTime {
	  return sql.NullTime{Time: t, Valid: !t.IsZero()}
   }
