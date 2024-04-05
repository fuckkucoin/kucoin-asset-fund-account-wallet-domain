package claimmutex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLStore SQL 存储实现。
type SQLStore struct {
	db *sql.DB
	dialect string // "mysql" | "postgres"
}

// NewSQLStore 创建 SQL 存储。
// dialect 应为 "mysql" 或 "postgres"。

  func NewSQLStore(db *sql.DB, dialect string) *SQLStore {
	   return &SQLStore{db: db, dialect: dialect}
   }

// ── OutputLease ──────────────────────────────────────────────────────────────

   func (s *SQLStore) GetLease(ctx context.Context, key OutputKey) (*OutputLease, error) {
	  q := `SELECT lease_id, chain, tx_hash, output_index, generation, 
	             state, winner_kind, winner_actor, claim_case_id,
	             issued_at, expires_at, claimed_at, blocked_at, block_reason
	      FROM dc_output_leases
	      WHERE chain = ? AND tx_hash = ? AND output_index = ?`
	   row := s.db.QueryRowContext(ctx,
		   s.rewrite(q), key.Chain, key.TxHash, key.OutputIndex)
	  return scanLease(row)
   }

   func (s *SQLStore) GetLeaseByID(ctx context.Context, leaseID string) (*OutputLease, error) {
	  q := `SELECT lease_id, chain, tx_hash, output_index, generation,
	             state, winner_kind, winner_actor, claim_case_id,
	             issued_at, expires_at, claimed_at, blocked_at, block_reason
	      FROM dc_output_leases WHERE lease_id = ?`
	   row := s.db.QueryRowContext(ctx, s.rewrite(q), leaseID)
	   return scanLease(row)
  }

   func scanLease(row *sql.Row) (*OutputLease, error) {
	   var l OutputLease
	  var claimedAt, blockedAt sql.NullTime
	  var blockReason,
		   caseID sql.NullString
	  err := row.Scan(
		  &l.LeaseID, &l.OutputKey.Chain, &l.OutputKey.TxHash, &l.OutputKey.OutputIndex,
		  &l.Generation, &l.State, &l.WinnerKind, &l.WinnerActor, &caseID,
		   &l.IssuedAt, &l.ExpiresAt, &claimedAt, &blockedAt, &blockReason,
	   )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrLeaseNotFound
		  }
		   return nil, err
	  }
	  if claimedAt.Valid {
		  t := claimedAt.Time
		   l.ClaimedAt = &t
	  }
	  if blockedAt.Valid {
		  t := blockedAt.Time
		   l.BlockedAt = &t
	   }
	  if blockReason.Valid {
		   l.BlockReason = blockReason.String
	   }
	   if caseID.Valid {
		  l.ClaimCaseID = caseID.String
	  }
	  return &l, nil
  }

   func (s *SQLStore) InsertLease(ctx context.Context, lease OutputLease) error {
	  q := `INSERT INTO dc_output_leases
	      (lease_id, chain, tx_hash, output_index, generation, state,
	       winner_kind, winner_actor, claim_case_id, issued_at, expires_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		  lease.LeaseID, lease.OutputKey.Chain, lease.OutputKey.TxHash, lease.OutputKey.OutputIndex,
		   lease.Generation, string(lease.State), lease.WinnerKind, lease.WinnerActor,
		   nullStr(lease.ClaimCaseID), lease.IssuedAt, lease.ExpiresAt,
	   )
	   if err != nil {
		  if isDup(err) {
			  return ErrMutexConflict
		   }
		  return err
	  }
	   return nil
   }

  func (s *SQLStore) UpdateLeaseState(ctx context.Context, leaseID string, expected LeaseState, next LeaseState, now time.Time, blockReason string) error {
	  q := `UPDATE dc_output_leases SET state=?, blocked_at=?, block_reason=?, claimed_at=? 
	      WHERE lease_id=? AND state=?`
	   var blockedAt, claimedAt sql.NullTime
	  if next == LeaseBlocked {
		   blockedAt = sql.NullTime{Time: now, Valid: true}
	   }
	  if next == LeaseClaimed || next == LeaseAutoCredited {
		   claimedAt = sql.NullTime{Time: now, Valid: true}
	  }
	  res, err := s.db.ExecContext(ctx, s.rewrite(q),
		  string(next), blockedAt, nullStr(blockReason), claimedAt,
		   leaseID, string(expected),
	  )
	  if err != nil {
		   return err
	   }
	  return requireOneRow(res, ErrGenerationMismatch)
   }

   func (s *SQLStore) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]OutputLease, error) {
	  q := `SELECT lease_id, chain, tx_hash, output_index, generation,
	             state, winner_kind, winner_actor, claim_case_id,
	             issued_at, expires_at, claimed_at, blocked_at, block_reason
	      FROM dc_output_leases
	      WHERE state = 'open' AND expires_at < ?
	      LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), now, limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanLeaseRows(rows)
  }

   func scanLeaseRows(rows *sql.Rows) ([]OutputLease, error) {
	   var out []OutputLease
	   for rows.Next() {
		   var l OutputLease
		  var claimedAt,
			   blockedAt sql.NullTime
		   var blockReason, caseID sql.NullString
		   if err := rows.Scan(
			  &l.LeaseID,
			   &l.OutputKey.Chain, &l.OutputKey.TxHash, &l.OutputKey.OutputIndex,
			   &l.Generation,
			  &l.State, &l.WinnerKind, &l.WinnerActor, &caseID,
			   &l.IssuedAt, &l.ExpiresAt, &claimedAt, &blockedAt, &blockReason,
		  ); err != nil {
			  return nil, err
		   }
		   if claimedAt.Valid {
			  t := claimedAt.Time
			  l.ClaimedAt = &t
		   }
		  if blockedAt.Valid {
			  t := blockedAt.Time
			  l.BlockedAt = &t
		   }
		   if blockReason.Valid {
			   l.BlockReason = blockReason.String
		   }
		   if caseID.Valid {
			  l.ClaimCaseID = caseID.String
		   }
		   out = append(out, l)
	  }
	  return out, rows.Err()
  }

// ── DupClaimCase ─────────────────────────────────────────────────────────────

  func (s *SQLStore) GetOpenDupCase(ctx context.Context, key OutputKey) (*DupClaimCase, error) {
	  q := `SELECT case_id, chain, tx_hash, output_index, lease_id,
	             first_kind, first_actor, first_idem, second_kind, second_actor, second_idem,
	             delta_amount, state, detected_at, proven_at, closed_at,
	             actor1, actor2, close_note
	      FROM dc_dup_cases
	      WHERE chain=? AND tx_hash=? AND output_index=? AND state != 'closed'
	      LIMIT 1`
	   row := s.db.QueryRowContext(ctx, s.rewrite(q), key.Chain, key.TxHash, key.OutputIndex)
	   return scanDupCase(row)
  }

  func (s *SQLStore) GetDupCaseByID(ctx context.Context, caseID string) (*DupClaimCase, error) {
	   q := `SELECT case_id, chain, tx_hash, output_index, lease_id,
	             first_kind, first_actor, first_idem, second_kind, second_actor, second_idem,
	             delta_amount, state, detected_at, proven_at, closed_at,
	             actor1, actor2, close_note
	      FROM dc_dup_cases WHERE case_id=?`
	  row := s.db.QueryRowContext(ctx, s.rewrite(q), caseID)
	  return scanDupCase(row)
   }

  func scanDupCase(row *sql.Row) (*DupClaimCase, error) {
	  var c DupClaimCase
	  var provenAt, closedAt sql.NullTime
	  var actor1, actor2, closeNote, firstIdem, secondIdem sql.NullString
	  err := row.Scan(
		   &c.CaseID, &c.OutputKey.Chain, &c.OutputKey.TxHash, &c.OutputKey.OutputIndex,
		   &c.LeaseID,
		   &c.FirstKind, &c.FirstActor, &firstIdem,
		  &c.SecondKind,
		   &c.SecondActor, &secondIdem,
		  &c.DeltaAmount, &c.State, &c.DetectedAt, &provenAt, &closedAt,
		   &actor1, &actor2, &closeNote,
	  )
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrDupCaseNotFound
		  }
		  return nil, err
	   }
	  if provenAt.Valid {
		   t := provenAt.Time
		  c.ProvenAt = &t
	  }
	  if closedAt.Valid {
		   t := closedAt.Time
		  c.ClosedAt = &t
	   }
	  if actor1.Valid {
		   c.Actor1 = actor1.String
	  }
	  if actor2.Valid {
		  c.Actor2 = actor2.String
	  }
	   if closeNote.Valid {
		  c.CloseNote = closeNote.String
	   }
	   if firstIdem.Valid {
		  c.FirstIdem = firstIdem.String
	   }
	   if secondIdem.Valid {
		   c.SecondIdem = secondIdem.String
	   }
	  return &c, nil
   }

  func (s *SQLStore) InsertDupCase(ctx context.Context, c DupClaimCase) error {
	   q := `INSERT INTO dc_dup_cases
	      (case_id, chain, tx_hash, output_index, lease_id,
	       first_kind, first_actor, first_idem, second_kind, second_actor, second_idem,
	       delta_amount, state, detected_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   c.CaseID, c.OutputKey.Chain, c.OutputKey.TxHash, c.OutputKey.OutputIndex,
		   c.LeaseID, c.FirstKind, c.FirstActor, nullStr(c.FirstIdem),
		  c.SecondKind, c.SecondActor, nullStr(c.SecondIdem),
		  c.DeltaAmount, string(c.State), c.DetectedAt,
	  )
	  return err
  }

  func (s *SQLStore) UpdateDupCaseState(ctx context.Context, caseID string, expected DupCaseState, next DupCaseState, now time.Time, actor1, actor2, closeNote string) error {
	   q := `UPDATE dc_dup_cases SET state=?, proven_at=?, closed_at=?, actor1=?, actor2=?, close_note=?
	      WHERE case_id=? AND state=?`
	   var provenAt, closedAt sql.NullTime
	  if next == DupCaseProven {
		   provenAt = sql.NullTime{Time: now, Valid: true}
	  }
	   if next == DupCaseClosed {
		  closedAt = sql.NullTime{Time: now, Valid: true}
	  }
	  res, err := s.db.ExecContext(ctx, s.rewrite(q),
		   string(next), provenAt, closedAt,
		  nullStr(actor1),
		   nullStr(actor2), nullStr(closeNote),
		   caseID, string(expected),
	  )
	   if err != nil {
		   return err
	   }
	   return requireOneRow(res,
		   ErrGenerationMismatch)
  }

  func (s *SQLStore) ListOpenDupCases(ctx context.Context, limit int) ([]DupClaimCase, error) {
	  q := `SELECT case_id, chain, tx_hash, output_index, lease_id,
	             first_kind, first_actor, first_idem, second_kind, second_actor, second_idem,
	             delta_amount, state, detected_at, proven_at, closed_at,
	             actor1, actor2, close_note
	      FROM dc_dup_cases WHERE state='open' LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), limit)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []DupClaimCase
	  for rows.Next() {
		  var c DupClaimCase
		   var provenAt, closedAt sql.NullTime
		  var actor1, actor2, closeNote, firstIdem, secondIdem sql.NullString
		   if err := rows.Scan(
			   &c.CaseID, &c.OutputKey.Chain, &c.OutputKey.TxHash, &c.OutputKey.OutputIndex,
			  &c.LeaseID,
			  &c.FirstKind, &c.FirstActor, &firstIdem,
			  &c.SecondKind, &c.SecondActor, &secondIdem,
			  &c.DeltaAmount, &c.State, &c.DetectedAt, &provenAt, &closedAt,
			  &actor1, &actor2, &closeNote,
		  ); err != nil {
			   return nil, err
		  }
		  if provenAt.Valid {
			  t := provenAt.Time
			   c.ProvenAt = &t
		  }
		  if actor1.Valid {
			   c.Actor1 = actor1.String
		  }
		   if actor2.Valid {
			  c.Actor2 = actor2.String
		   }
		   if firstIdem.Valid {
			   c.FirstIdem = firstIdem.String
		   }
		  if secondIdem.Valid {
			  c.SecondIdem = secondIdem.String
		  }
		  out = append(out, c)
	   }
	   return out, rows.Err()
  }

// ── AutoCreditBind ───────────────────────────────────────────────────────────

   func (s *SQLStore) GetPendingAutoBind(ctx context.Context, key OutputKey) (*AutoCreditBind, error) {
	  q := `SELECT bind_id, chain, tx_hash, output_index, generation, lease_id,
	             user_id, asset, amount, idem_key, state, bound_at, resolved_at, reject_note
	      FROM dc_auto_binds
	      WHERE chain=? AND tx_hash=? AND output_index=? AND state='pending'
	      LIMIT 1`
	  row := s.db.QueryRowContext(ctx, s.rewrite(q), key.Chain, key.TxHash, key.OutputIndex)
	   return scanAutoBind(row)
   }

  func (s *SQLStore) GetAutoBindByID(ctx context.Context, bindID string) (*AutoCreditBind, error) {
	   q := `SELECT bind_id, chain, tx_hash, output_index, generation, lease_id,
	             user_id, asset, amount, idem_key, state, bound_at, resolved_at, reject_note
	      FROM dc_auto_binds WHERE bind_id=?`
	  row := s.db.QueryRowContext(ctx, s.rewrite(q), bindID)
	   return scanAutoBind(row)
   }

  func scanAutoBind(row *sql.Row) (*AutoCreditBind, error) {
	  var b AutoCreditBind
	  var resolvedAt sql.NullTime
	   var rejectNote sql.NullString
	   err := row.Scan(
		  &b.BindID, &b.OutputKey.Chain, &b.OutputKey.TxHash, &b.OutputKey.OutputIndex,
		   &b.Generation, &b.LeaseID, &b.UserID, &b.Asset, &b.Amount, &b.IdemKey,
		  &b.State, &b.BoundAt, &resolvedAt, &rejectNote,
	  )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrBindNotFound
		   }
		   return nil, err
	   }
	  if resolvedAt.Valid {
		   t := resolvedAt.Time
		  b.ResolvedAt = &t
	  }
	  if rejectNote.Valid {
		  b.RejectNote = rejectNote.String
	  }
	   return &b, nil
   }

  func (s *SQLStore) InsertAutoBind(ctx context.Context, b AutoCreditBind) error {
	  q := `INSERT INTO dc_auto_binds
	      (bind_id, chain, tx_hash, output_index, generation, lease_id,
	       user_id, asset, amount, idem_key, state, bound_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q),
		  b.BindID,
		   b.OutputKey.Chain, b.OutputKey.TxHash, b.OutputKey.OutputIndex,
		  b.Generation, b.LeaseID, b.UserID, b.Asset, b.Amount, b.IdemKey,
		   string(b.State), b.BoundAt,
	   )
	  return err
   }

  func (s *SQLStore) UpdateAutoBindState(ctx context.Context, bindID string, expected BindState, next BindState, now time.Time, rejectNote string) error {
	  q := `UPDATE dc_auto_binds SET state=?, resolved_at=?, reject_note=?
	      WHERE bind_id=? AND state=?`
	  res, err := s.db.ExecContext(ctx, s.rewrite(q),
		   string(next), now, nullStr(rejectNote),
		  bindID, string(expected),
	  )
	   if err != nil {
		   return err
	   }
	  return requireOneRow(res, ErrGenerationMismatch)
  }

// ── ClaimEffectProof ─────────────────────────────────────────────────────────

   func (s *SQLStore) GetProofByOutput(ctx context.Context, key OutputKey) (*ClaimEffectProof, error) {
	  q := `SELECT proof_id, chain, tx_hash, output_index, lease_id, generation, 
	             claim_kind, actor_id, user_id, asset, amount, idem_key,
	             state, created_at, verified_at, sealed_at, fail_reason, sealed_by
	      FROM dc_effect_proofs
	      WHERE chain=? AND tx_hash=? AND output_index=?
	      ORDER BY created_at DESC LIMIT 1`
	  row := s.db.QueryRowContext(ctx,
		   s.rewrite(q), key.Chain, key.TxHash, key.OutputIndex)
	   return scanProof(row)
   }

   func (s *SQLStore) GetProofByID(ctx context.Context, proofID string) (*ClaimEffectProof, error) {
	  q := `SELECT proof_id, chain, tx_hash, output_index, lease_id, generation,
	             claim_kind, actor_id, user_id, asset, amount, idem_key,
	             state, created_at, verified_at, sealed_at, fail_reason, sealed_by
	      FROM dc_effect_proofs WHERE proof_id=?`
	   row := s.db.QueryRowContext(ctx, s.rewrite(q), proofID)
	   return scanProof(row)
   }

  func scanProof(row *sql.Row) (*ClaimEffectProof, error) {
	   var p ClaimEffectProof
	  var verifiedAt, sealedAt sql.NullTime
	  var failReason, sealedBy sql.NullString
	  err := row.Scan(
		  &p.ProofID, &p.OutputKey.Chain, &p.OutputKey.TxHash, &p.OutputKey.OutputIndex,
		  &p.LeaseID, &p.Generation, &p.ClaimKind, &p.ActorID, &p.UserID,
		  &p.Asset, &p.Amount, &p.IdemKey,
		  &p.State, &p.CreatedAt, &verifiedAt, &sealedAt, &failReason, &sealedBy,
	  )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrProofNotFound
		  }
		  return nil, err
	  }
	  if verifiedAt.Valid {
		   t := verifiedAt.Time
		   p.VerifiedAt = &t
	   }
	  if sealedAt.Valid {
		   t := sealedAt.Time
		  p.SealedAt = &t
	   }
	   if failReason.Valid {
		  p.FailReason = failReason.String
	   }
	  if sealedBy.Valid {
		   p.SealedBy = sealedBy.String
	  }
	   return &p, nil
   }

   func (s *SQLStore) InsertProof(ctx context.Context, p ClaimEffectProof) error {
	  q := `INSERT INTO dc_effect_proofs
	      (proof_id, chain, tx_hash, output_index, lease_id, generation,
	       claim_kind, actor_id, user_id, asset, amount, idem_key, state, created_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   p.ProofID, p.OutputKey.Chain, p.OutputKey.TxHash, p.OutputKey.OutputIndex,
		  p.LeaseID,
		   p.Generation, p.ClaimKind, p.ActorID, p.UserID,
		   p.Asset, p.Amount, p.IdemKey, string(p.State), p.CreatedAt,
	  )
	   return err
   }

  func (s *SQLStore) UpdateProofState(ctx context.Context, proofID string, expected ProofState, next ProofState, now time.Time, failReason, sealedBy string) error {
	  q := `UPDATE dc_effect_proofs SET state=?, verified_at=?, sealed_at=?, fail_reason=?, sealed_by=?
	      WHERE proof_id=? AND state=?`
	   var verifiedAt, sealedAt sql.NullTime
	   if next == ProofVerified {
		   verifiedAt = sql.NullTime{Time: now, Valid: true}
	   }
	   if next == ProofSealed {
		  sealedAt = sql.NullTime{Time: now, Valid: true}
	   }
	  res, err := s.db.ExecContext(ctx, s.rewrite(q),
		   string(next), verifiedAt, sealedAt,
		   nullStr(failReason),
		   nullStr(sealedBy),
		   proofID, string(expected),
	  )
	  if err != nil {
		  return err
	  }
	   return requireOneRow(res,
		   ErrGenerationMismatch)
   }

   func (s *SQLStore) ListPendingProofs(ctx context.Context, limit int) ([]ClaimEffectProof, error) {
	  q := `SELECT proof_id, chain, tx_hash, output_index, lease_id, generation,
	             claim_kind, actor_id, user_id, asset, amount, idem_key,
	             state, created_at, verified_at, sealed_at, fail_reason, sealed_by
	      FROM dc_effect_proofs WHERE state='pending' LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), limit)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []ClaimEffectProof
	   for rows.Next() {
		   var p ClaimEffectProof
		   var verifiedAt, sealedAt sql.NullTime
		  var failReason, sealedBy sql.NullString
		  if err := rows.Scan(
			  &p.ProofID,
			   &p.OutputKey.Chain, &p.OutputKey.TxHash, &p.OutputKey.OutputIndex,
			  &p.LeaseID, &p.Generation, &p.ClaimKind, &p.ActorID, &p.UserID,
			   &p.Asset, &p.Amount, &p.IdemKey,
			  &p.State, &p.CreatedAt, &verifiedAt, &sealedAt, &failReason, &sealedBy,
		  ); err != nil {
			  return nil, err
		  }
		  if verifiedAt.Valid {
			   t := verifiedAt.Time
			  p.VerifiedAt = &t
		  }
		  if failReason.Valid {
			   p.FailReason = failReason.String
		  }
		   out = append(out, p)
	  }
	  return out, rows.Err()
  }

// ── helpers ──────────────────────────────────────────────────────────────────

  func (s *SQLStore) rewrite(q string) string {
	   if s.dialect != "postgres" {
		   return q
	  }
	   // 将 MySQL 风格 ? 替换为 $1,$2,... for PostgreSQL
	  n := 0
	  out := make([]byte, 0, len(q)+16)
	   for i := 0; i < len(q); i++ {
		   if q[i] == '?' {
			  n++
			  out = append(out, '$')
			   out = append(out, []byte(fmt.Sprintf("%d", n))...)
		   } else {
			  out = append(out, q[i])
		   }
	   }
	  return string(out)
  }

  func nullStr(s string) sql.NullString {
	  if s == "" {
		   return sql.NullString{}
	   }
	  return sql.NullString{String: s,
		  Valid: true}
  }

  func requireOneRow(res sql.Result, notFoundErr error) error {
	  n, err := res.RowsAffected()
	   if err != nil {
		   return err
	  }
	   if n == 0 {
		   return notFoundErr
	  }
	   return nil
  }

   func isDup(err error) bool {
	  if err == nil {
		  return false
	   }
	  msg := err.Error()
	   return contains(msg, "Duplicate entry") || contains(msg, "duplicate key") || contains(msg, "UNIQUE constraint failed")
   }

   func contains(s, sub string) bool {
	   return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s,
		   sub))
   }

   func containsStr(s, sub string) bool {
	   for i := 0; i <= len(s)-len(sub); i++ {
		  if s[i:i+len(sub)] == sub {
			  return true
		  }
	  }
	   return false
   }
