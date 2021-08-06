// @FileName: sqlstore.go
// @Summary: Store 接口的 SQL（MySQL）持久化实现。
// - 读写 keycer_rotate_leases、keycer_sign_binds、keycer_rot_drift_cases、keycer_fp_proofs 四张表。
// - SaveLease / SaveSignBind / SaveDriftCase / SaveProof 使用 INSERT IGNORE 保证幂等。
// - UpdateLeaseState 通过 UPDATE … WHERE state=from 实现乐观锁（CAS）。
// - 不依赖 ORM；使用 database/sql 口径接口。
// 不变量：
// - 同 ticket_id 的 keycer_rotate_leases 最多一条非终态租约（业务层保证）。
// - keycer_sign_binds.sign_bind_id 主键唯一；同 operation_id 最多一条。
// - keycer_fp_proofs 按 rotate_lease_id 唯一。
// @Author: Draven
// @Created: 2021.08.05

package keyrotgate

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

// ── RotateLease ──────────────────────────────────────────────────────────────

  func (s *SQLStore) SaveLease(ctx context.Context, l *RotateLease) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	  approvedAt := nullTime(l.ApprovedAt)
	   executingAt := nullTime(l.ExecutingAt)
	  doneAt := nullTime(l.DoneAt)
	  blockedAt := nullTime(l.BlockedAt)
	   _, err := s.db.ExecContext(ctx,
		   `INSERT INTO keycer_rotate_leases
		 (rotate_lease_id, ticket_id, src_slot_id, dst_slot_id,
		  src_fingerprint, dst_fingerprint, rotate_gen,
		  state, opened_by, approved_by, executed_by, note,
		  opened_at, expires_at, approved_at, executing_at, done_at, blocked_at,
		  block_reason, proof_id, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   state=VALUES(state), approved_by=VALUES(approved_by),
		   executed_by=VALUES(executed_by),
		   approved_at=VALUES(approved_at), executing_at=VALUES(executing_at),
		   done_at=VALUES(done_at), blocked_at=VALUES(blocked_at),
		   block_reason=VALUES(block_reason), proof_id=VALUES(proof_id),
		   updated_at=VALUES(updated_at)`,
		   l.RotateLeaseID,
		  l.TicketID, l.SrcSlotID, l.DstSlotID,
		   l.SrcFingerprint, l.DstFingerprint, l.RotateGen,
		  string(l.State), l.OpenedBy, l.ApprovedBy, l.ExecutedBy, l.Note,
		  l.OpenedAt, l.ExpiresAt, approvedAt, executingAt, doneAt, blockedAt,
		   l.BlockReason, l.ProofID, l.CreatedAt, l.UpdatedAt,
	   )
	   return err
  }

  func (s *SQLStore) GetLease(ctx context.Context, id string) (*RotateLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx,
		   `SELECT rotate_lease_id, ticket_id, src_slot_id, dst_slot_id,
		        src_fingerprint, dst_fingerprint, rotate_gen,
		        state, opened_by, approved_by, executed_by, note,
		        opened_at, expires_at, approved_at, executing_at, done_at, blocked_at,
		        block_reason, proof_id, created_at, updated_at
		   FROM keycer_rotate_leases WHERE rotate_lease_id = ?`, id)
	   return scanLease(row)
  }

  func (s *SQLStore) GetLeaseByTicket(ctx context.Context, ticketID string) (*RotateLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx,
		   `SELECT rotate_lease_id, ticket_id, src_slot_id, dst_slot_id, 
		        src_fingerprint, dst_fingerprint, rotate_gen,
		        state, opened_by, approved_by, executed_by, note,
		        opened_at, expires_at, approved_at, executing_at, done_at, blocked_at,
		        block_reason, proof_id, created_at, updated_at
		   FROM keycer_rotate_leases WHERE ticket_id = ? LIMIT 1`, ticketID)
	   return scanLease(row)
  }

  func (s *SQLStore) ListActiveLeasesBySlot(ctx context.Context, srcSlotID string) ([]*RotateLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   rows,
		  err := s.db.QueryContext(ctx,
		  `SELECT rotate_lease_id, ticket_id, src_slot_id, dst_slot_id, 
		        src_fingerprint, dst_fingerprint, rotate_gen,
		        state, opened_by, approved_by, executed_by, note,
		        opened_at, expires_at, approved_at, executing_at, done_at, blocked_at,
		        block_reason, proof_id, created_at, updated_at
		   FROM keycer_rotate_leases
		  WHERE src_slot_id = ? AND state NOT IN ('done','blocked')`, srcSlotID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanLeases(rows)
  }

   func (s *SQLStore) UpdateLeaseState(ctx context.Context, id string, from, to LeaseState, update *RotateLease) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	   approvedAt := nullTime(update.ApprovedAt)
	   executingAt := nullTime(update.ExecutingAt)
	   doneAt := nullTime(update.DoneAt)
	  blockedAt := nullTime(update.BlockedAt)
	  res, err := s.db.ExecContext(ctx,
		   `UPDATE keycer_rotate_leases 
		    SET state=?, approved_by=?, executed_by=?,
		        approved_at=?, executing_at=?, done_at=?, blocked_at=?,
		        block_reason=?, proof_id=?, updated_at=?
		  WHERE rotate_lease_id=? AND state=?`,
		  string(to), update.ApprovedBy, update.ExecutedBy,
		  approvedAt, executingAt, doneAt, blockedAt,
		  update.BlockReason, update.ProofID, update.UpdatedAt,
		  id, string(from),
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

   func (s *SQLStore) ListExpiredLeases(ctx context.Context, limit int) ([]*RotateLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 200
	  }
	  rows, err := s.db.QueryContext(ctx,
		  `SELECT rotate_lease_id, ticket_id, src_slot_id, dst_slot_id,
		        src_fingerprint, dst_fingerprint, rotate_gen,
		        state, opened_by, approved_by, executed_by, note,
		        opened_at, expires_at, approved_at, executing_at, done_at, blocked_at,
		        block_reason, proof_id, created_at, updated_at
		   FROM keycer_rotate_leases
		  WHERE state NOT IN ('done','blocked') AND expires_at < NOW(3)
		  LIMIT ?`, limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanLeases(rows)
  }

// ── SignBind ──────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveSignBind(ctx context.Context, b *SignBind) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  releasedAt := nullTime(b.ReleasedAt)
	  expiredAt := nullTime(b.ExpiredAt)
	  rejectedAt := nullTime(b.RejectedAt)
	  _, err := s.db.ExecContext(ctx,
		   `INSERT INTO keycer_sign_binds
		 (sign_bind_id, src_slot_id, fingerprint, operation_id, operation_kind,
		  bound_by, state, bound_at, expires_at,
		  released_at, expired_at, rejected_at, drift_case_id, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   state=VALUES(state),
		   released_at=VALUES(released_at), expired_at=VALUES(expired_at),
		   rejected_at=VALUES(rejected_at), drift_case_id=VALUES(drift_case_id),
		   updated_at=VALUES(updated_at)`,
		  b.SignBindID, b.SrcSlotID, b.Fingerprint, b.OperationID, b.OperationKind,
		   b.BoundBy, string(b.State), b.BoundAt, b.ExpiresAt,
		   releasedAt, expiredAt, rejectedAt, b.DriftCaseID, b.CreatedAt, b.UpdatedAt,
	  )
	  return err
  }

  func (s *SQLStore) GetSignBind(ctx context.Context, id string) (*SignBind, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx,
		  `SELECT sign_bind_id, src_slot_id, fingerprint, operation_id, operation_kind,
		        bound_by, state, bound_at, expires_at,
		        released_at, expired_at, rejected_at, drift_case_id, created_at, updated_at
		   FROM keycer_sign_binds WHERE sign_bind_id = ?`, id)
	   return scanSignBind(row)
  }

  func (s *SQLStore) GetSignBindByOperation(ctx context.Context, operationID string) (*SignBind, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx,
		  `SELECT sign_bind_id, src_slot_id, fingerprint, operation_id, operation_kind,
		        bound_by, state, bound_at, expires_at,
		        released_at, expired_at, rejected_at, drift_case_id, created_at, updated_at
		   FROM keycer_sign_binds WHERE operation_id = ? LIMIT 1`, operationID)
	  return scanSignBind(row)
   }

  func (s *SQLStore) ListActiveSignBinds(ctx context.Context,
	  srcSlotID string) ([]*SignBind, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  var rows *sql.Rows
	   var err error
	   if strings.TrimSpace(srcSlotID) == "" {
		   rows, err = s.db.QueryContext(ctx,
			   `SELECT sign_bind_id, src_slot_id, fingerprint, operation_id, operation_kind,
			        bound_by, state, bound_at, expires_at,
			        released_at, expired_at, rejected_at, drift_case_id, created_at, updated_at
			   FROM keycer_sign_binds
			  WHERE state='active' AND expires_at > NOW(3)`)
	   } else {
		  rows, err = s.db.QueryContext(ctx,
			   `SELECT sign_bind_id, src_slot_id, fingerprint, operation_id, operation_kind, 
			        bound_by, state, bound_at, expires_at,
			        released_at, expired_at, rejected_at, drift_case_id, created_at, updated_at
			   FROM keycer_sign_binds
			  WHERE src_slot_id=? AND state='active' AND expires_at > NOW(3)`, srcSlotID)
	   }
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanSignBinds(rows)
  }

   func (s *SQLStore) UpdateSignBindState(ctx context.Context, id string, from, to SignBindState, update *SignBind) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	  releasedAt := nullTime(update.ReleasedAt)
	   expiredAt := nullTime(update.ExpiredAt)
	   rejectedAt := nullTime(update.RejectedAt)
	   res, err := s.db.ExecContext(ctx,
		   `UPDATE keycer_sign_binds
		    SET state=?, released_at=?, expired_at=?, rejected_at=?,
		        drift_case_id=?, updated_at=?
		  WHERE sign_bind_id=? AND state=?`,
		  string(to), releasedAt, expiredAt, rejectedAt,
		  update.DriftCaseID,
		   update.UpdatedAt,
		   id, string(from),
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

  func (s *SQLStore) ListExpiredSignBinds(ctx context.Context, limit int) ([]*SignBind, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   var rows *sql.Rows
	  var err error
	  if limit <= 0 {
		   rows, err = s.db.QueryContext(ctx,
			  `SELECT sign_bind_id, src_slot_id, fingerprint, operation_id, operation_kind,
			        bound_by, state, bound_at, expires_at,
			        released_at, expired_at, rejected_at, drift_case_id, created_at, updated_at
			   FROM keycer_sign_binds
			  WHERE state='active' AND expires_at < NOW(3)`)
	  } else {
		   rows,
			   err = s.db.QueryContext(ctx,
			   `SELECT sign_bind_id, src_slot_id, fingerprint, operation_id, operation_kind,
			        bound_by, state, bound_at, expires_at,
			        released_at, expired_at, rejected_at, drift_case_id, created_at, updated_at
			   FROM keycer_sign_binds
			  WHERE state='active' AND expires_at < NOW(3)
			  LIMIT ?`, limit)
	   }
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanSignBinds(rows)
  }

// ── KeyRotDriftCase ───────────────────────────────────────────────────────────

  func (s *SQLStore) SaveDriftCase(ctx context.Context, d *KeyRotDriftCase) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	   provenAt := nullTime(d.ProvenAt)
	   closedAt := nullTime(d.ClosedAt)
	   _, err := s.db.ExecContext(ctx,
		  `INSERT INTO keycer_rot_drift_cases
		 (drift_case_id, src_slot_id, old_fingerprint, new_fingerprint,
		  sign_bind_id, rotate_lease_id, state,
		  drift_detected_at, proven_at, closed_at,
		  proof_checksum, confirmed_by, co_confirmed_by, note,
		  created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   state=VALUES(state), proven_at=VALUES(proven_at), closed_at=VALUES(closed_at),
		   proof_checksum=VALUES(proof_checksum),
		   confirmed_by=VALUES(confirmed_by), co_confirmed_by=VALUES(co_confirmed_by),
		   note=VALUES(note), updated_at=VALUES(updated_at)`,
		  d.DriftCaseID, d.SrcSlotID, d.OldFingerprint, d.NewFingerprint,
		   d.SignBindID,
		   d.RotateLeaseID, string(d.State),
		  d.DriftDetectedAt, provenAt, closedAt,
		  d.ProofChecksum, d.ConfirmedBy, d.CoConfirmedBy, d.Note,
		  d.CreatedAt, d.UpdatedAt,
	   )
	  return err
  }

  func (s *SQLStore) GetDriftCase(ctx context.Context, id string) (*KeyRotDriftCase, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx,
		  `SELECT drift_case_id, src_slot_id, old_fingerprint, new_fingerprint, 
		        sign_bind_id, rotate_lease_id, state,
		        drift_detected_at, proven_at, closed_at,
		        proof_checksum, confirmed_by, co_confirmed_by, note,
		        created_at, updated_at
		   FROM keycer_rot_drift_cases WHERE drift_case_id = ?`, id)
	   return scanDriftCase(row)
  }

   func (s *SQLStore) ListOpenDriftCases(ctx context.Context, srcSlotID string) ([]*KeyRotDriftCase, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  var rows *sql.Rows
	   var err error
	   if strings.TrimSpace(srcSlotID) == "" {
		   rows, err = s.db.QueryContext(ctx,
			   `SELECT drift_case_id, src_slot_id, old_fingerprint, new_fingerprint,
			        sign_bind_id, rotate_lease_id, state,
			        drift_detected_at, proven_at, closed_at,
			        proof_checksum, confirmed_by, co_confirmed_by, note,
			        created_at, updated_at
			   FROM keycer_rot_drift_cases WHERE state != 'closed'`)
	   } else {
		   rows, err = s.db.QueryContext(ctx,
			   `SELECT drift_case_id, src_slot_id, old_fingerprint, new_fingerprint,
			        sign_bind_id, rotate_lease_id, state,
			        drift_detected_at, proven_at, closed_at,
			        proof_checksum, confirmed_by, co_confirmed_by, note,
			        created_at, updated_at
			   FROM keycer_rot_drift_cases WHERE src_slot_id=? AND state != 'closed'`, srcSlotID)
	   }
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanDriftCases(rows)
  }

  func (s *SQLStore) UpdateDriftCaseState(ctx context.Context, id string, from, to DriftCaseState, update *KeyRotDriftCase) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	   provenAt := nullTime(update.ProvenAt)
	  closedAt := nullTime(update.ClosedAt)
	   res, err := s.db.ExecContext(ctx,
		  `UPDATE keycer_rot_drift_cases
		    SET state=?, proven_at=?, closed_at=?,
		        proof_checksum=?, confirmed_by=?, co_confirmed_by=?,
		        note=?, updated_at=?
		  WHERE drift_case_id=? AND state=?`,
		   string(to), provenAt, closedAt,
		  update.ProofChecksum, update.ConfirmedBy, update.CoConfirmedBy,
		  update.Note, update.UpdatedAt,
		   id, string(from),
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

// ── FingerprintProof ──────────────────────────────────────────────────────────

  func (s *SQLStore) SaveProof(ctx context.Context, p *FingerprintProof) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx,
		   `INSERT IGNORE INTO keycer_fp_proofs 
		 (proof_id, rotate_lease_id, ticket_id,
		  src_slot_id, dst_slot_id,
		  before_fingerprint, after_fingerprint, rotate_gen,
		  proof_checksum, issued_by, issued_at, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		   p.ProofID, p.RotateLeaseID, p.TicketID,
		   p.SrcSlotID, p.DstSlotID,
		   p.BeforeFingerprint, p.AfterFingerprint, p.RotateGen,
		   p.ProofChecksum, p.IssuedBy, p.IssuedAt, p.CreatedAt,
	   )
	  return err
   }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (*FingerprintProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx,
		  `SELECT proof_id, rotate_lease_id, ticket_id,
		        src_slot_id, dst_slot_id,
		        before_fingerprint, after_fingerprint, rotate_gen,
		        proof_checksum, issued_by, issued_at, created_at
		   FROM keycer_fp_proofs WHERE proof_id = ?`, id)
	   return scanProof(row)
   }

  func (s *SQLStore) GetProofByLease(ctx context.Context, leaseID string) (*FingerprintProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx,
		  `SELECT proof_id, rotate_lease_id, ticket_id,
		        src_slot_id, dst_slot_id,
		        before_fingerprint, after_fingerprint, rotate_gen,
		        proof_checksum, issued_by, issued_at, created_at
		   FROM keycer_fp_proofs WHERE rotate_lease_id = ? LIMIT 1`, leaseID)
	  return scanProof(row)
  }

   func (s *SQLStore) GetLatestProofBySlot(ctx context.Context, srcSlotID string) (*FingerprintProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx,
		  `SELECT proof_id, rotate_lease_id, ticket_id,
		        src_slot_id, dst_slot_id,
		        before_fingerprint, after_fingerprint, rotate_gen,
		        proof_checksum, issued_by, issued_at, created_at
		   FROM keycer_fp_proofs WHERE src_slot_id = ?
		  ORDER BY issued_at DESC LIMIT 1`, srcSlotID)
	   return scanProof(row)
   }

// ── scan 辅助函数 ─────────────────────────────────────────────────────────────

type rower interface {
	Scan(dest ...any) error
}

   func scanLease(row rower) (*RotateLease, error) {
	  var l RotateLease
	   var approvedBy, executedBy, blockReason, proofID, note sql.NullString
	   var approvedAt, executingAt, doneAt, blockedAt sql.NullTime
	  err := row.Scan(
		   &l.RotateLeaseID, &l.TicketID, &l.SrcSlotID, &l.DstSlotID,
		  &l.SrcFingerprint, &l.DstFingerprint, &l.RotateGen,
		   &l.State, &l.OpenedBy, &approvedBy, &executedBy, &note,
		   &l.OpenedAt, &l.ExpiresAt, &approvedAt, &executingAt, &doneAt, &blockedAt,
		  &blockReason, &proofID, &l.CreatedAt, &l.UpdatedAt,
	   )
	   if err == sql.ErrNoRows {
		   return nil, ErrLeaseNotFound
	   }
	   if err != nil {
		  return nil, err
	  }
	  l.ApprovedBy = approvedBy.String
	   l.ExecutedBy = executedBy.String
	  l.Note = note.String
	  l.BlockReason = blockReason.String
	   l.ProofID = proofID.String
	   if approvedAt.Valid {
		   t := approvedAt.Time
		   l.ApprovedAt = &t
	   }
	   if executingAt.Valid {
		  t := executingAt.Time
		  l.ExecutingAt = &t
	  }
	  if doneAt.Valid {
		  t := doneAt.Time
		  l.DoneAt = &t
	  }
	  if blockedAt.Valid {
		  t := blockedAt.Time
		  l.BlockedAt = &t
	  }
	   return &l, nil
  }

  func scanLeases(rows *sql.Rows) ([]*RotateLease, error) {
	  var out []*RotateLease
	  for rows.Next() {
		  l, err := scanLease(rows)
		   if err != nil {
			   return nil, err
		  }
		   out = append(out, l)
	   }
	  return out, rows.Err()
  }

  func scanSignBind(row rower) (*SignBind, error) {
	  var b SignBind
	  var releasedAt, expiredAt, rejectedAt sql.NullTime
	   var driftCaseID sql.NullString
	   err := row.Scan(
		  &b.SignBindID, &b.SrcSlotID, &b.Fingerprint, &b.OperationID, &b.OperationKind,
		   &b.BoundBy, &b.State, &b.BoundAt, &b.ExpiresAt,
		   &releasedAt, &expiredAt, &rejectedAt, &driftCaseID, &b.CreatedAt, &b.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		   return nil, ErrSignBindNotFound
	   }
	   if err != nil {
		  return nil, err
	   }
	  b.DriftCaseID = driftCaseID.String
	   if releasedAt.Valid {
		  t := releasedAt.Time
		  b.ReleasedAt = &t
	  }
	  if expiredAt.Valid {
		  t := expiredAt.Time
		   b.ExpiredAt = &t
	   }
	   if rejectedAt.Valid {
		  t := rejectedAt.Time
		   b.RejectedAt = &t
	  }
	   return &b, nil
   }

   func scanSignBinds(rows *sql.Rows) ([]*SignBind, error) {
	   var out []*SignBind
	   for rows.Next() {
		   b, err := scanSignBind(rows)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, b)
	   }
	   return out, rows.Err()
   }

   func scanDriftCase(row rower) (*KeyRotDriftCase, error) {
	  var d KeyRotDriftCase
	   var provenAt, closedAt sql.NullTime
	  var proofChecksum, confirmedBy, coConfirmedBy, note, rotateLeaseID sql.NullString
	   err := row.Scan(
		   &d.DriftCaseID,
		   &d.SrcSlotID, &d.OldFingerprint, &d.NewFingerprint,
		   &d.SignBindID, &rotateLeaseID, &d.State,
		   &d.DriftDetectedAt, &provenAt, &closedAt,
		   &proofChecksum, &confirmedBy, &coConfirmedBy, &note,
		   &d.CreatedAt, &d.UpdatedAt,
	   )
	  if err == sql.ErrNoRows {
		  return nil, ErrDriftCaseNotFound
	  }
	   if err != nil {
		  return nil, err
	   }
	   d.RotateLeaseID = rotateLeaseID.String
	  d.ProofChecksum = proofChecksum.String
	  d.ConfirmedBy = confirmedBy.String
	  d.CoConfirmedBy = coConfirmedBy.String
	  d.Note = note.String
	  if provenAt.Valid {
		  t := provenAt.Time
		   d.ProvenAt = &t
	   }
	  if closedAt.Valid {
		   t := closedAt.Time
		   d.ClosedAt = &t
	   }
	  return &d, nil
  }

  func scanDriftCases(rows *sql.Rows) ([]*KeyRotDriftCase, error) {
	   var out []*KeyRotDriftCase
	   for rows.Next() {
		  d, err := scanDriftCase(rows)
		   if err != nil {
			  return nil, err
		   }
		   out = append(out, d)
	   }
	  return out, rows.Err()
  }

   func scanProof(row rower) (*FingerprintProof, error) {
	   var p FingerprintProof
	   err := row.Scan(
		   &p.ProofID, &p.RotateLeaseID, &p.TicketID,
		  &p.SrcSlotID, &p.DstSlotID,
		   &p.BeforeFingerprint, &p.AfterFingerprint, &p.RotateGen,
		   &p.ProofChecksum, &p.IssuedBy, &p.IssuedAt, &p.CreatedAt,
	   )
	   if err == sql.ErrNoRows {
		  return nil, ErrProofNotFound
	  }
	  if err != nil {
		  return nil, err
	  }
	   return &p, nil
  }

// ── 工具函数 ──────────────────────────────────────────────────────────────────

   func nullTime(t *time.Time) sql.NullTime {
	  if t == nil {
		  return sql.NullTime{}
	   }
	  return sql.NullTime{Time: *t, Valid: true}
   }
