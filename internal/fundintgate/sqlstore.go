package fundintgate

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore 基于 persist.SQLDB。
type SQLStore struct {
	db *persist.SQLDB
}

// NewSQLStore 构造 SQL 仓储。
  func NewSQLStore(db *persist.SQLDB) *SQLStore {
	   if db == nil {
		   panic("fundintgate: SQLStore requires non-nil SQLDB")
	   }
	   return &SQLStore{db: db}
   }

   func (s *SQLStore) q(ctx context.Context) persist.Queryer {
	   return s.db.Conn(ctx)
   }

   func (s *SQLStore) SaveLease(ctx context.Context, lease IntGateLease) error {
	  const q = `INSERT INTO int_gate_leases (
		lease_id, from_user_id, to_user_id, resolve_channel, to_hint, asset, amount,
		gate_fingerprint, gate_impl_id, gate_gen, halted, forbid_withdraw, state,
		ttl_seconds, issued_at, bound_at, sent_at, expired_at, revoked_at, revoke_reason,
		transfer_id, idempotency_key, send_idempotency_key, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.q(ctx).ExecContext(ctx, q,
		   lease.LeaseID, lease.FromUserID, lease.ToUserID, lease.ResolveChannel, lease.ToHint,
		   lease.Asset, lease.Amount, lease.GateFingerprint, lease.GateImplID, lease.GateGen,
		   boolToInt(lease.Halted), boolToInt(lease.ForbidWithdraw), string(lease.State),
		   lease.TTLSeconds, lease.IssuedAt, nullTime(lease.BoundAt), nullTime(lease.SentAt),
		   nullTime(lease.ExpiredAt), nullTime(lease.RevokedAt), lease.RevokeReason,
		   lease.TransferID, lease.IdempotencyKey, lease.SendIdempotencyKey,
		   lease.CreatedAt, lease.UpdatedAt,
	  )
	   if err != nil {
		   return WrapStore("save_lease", err)
	  }
	  return nil
   }

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (IntGateLease, error) {
	   const q = `SELECT lease_id, from_user_id, to_user_id, resolve_channel, to_hint, asset, amount,
		gate_fingerprint, gate_impl_id, gate_gen, halted, forbid_withdraw, state,
		ttl_seconds, issued_at, bound_at, sent_at, expired_at, revoked_at, revoke_reason,
		transfer_id, idempotency_key, send_idempotency_key, created_at, updated_at
		FROM int_gate_leases WHERE lease_id=?`
	  row := s.q(ctx).QueryRowContext(ctx, q, leaseID)
	  l, err := scanLease(row)
	   if err == sql.ErrNoRows {
		   return IntGateLease{},
			   ErrLeaseNotFound
	   }
	   if err != nil {
		  return IntGateLease{}, WrapStore("get_lease", err)
	   }
	  return l, nil
   }

  func (s *SQLStore) GetLeaseByIdempotencyKey(ctx context.Context, key string) (IntGateLease, bool, error) {
	  const q = `SELECT lease_id, from_user_id, to_user_id, resolve_channel, to_hint, asset, amount,
		gate_fingerprint, gate_impl_id, gate_gen, halted, forbid_withdraw, state,
		ttl_seconds, issued_at, bound_at, sent_at, expired_at, revoked_at, revoke_reason,
		transfer_id, idempotency_key, send_idempotency_key, created_at, updated_at
		FROM int_gate_leases WHERE idempotency_key=? LIMIT 1`
	   row := s.q(ctx).QueryRowContext(ctx, q, key)
	  l, err := scanLease(row)
	  if err == sql.ErrNoRows {
		   return IntGateLease{}, false, nil
	  }
	   if err != nil {
		  return IntGateLease{}, false, WrapStore("get_lease_idem", err)
	   }
	   return l, true, nil
  }

   func (s *SQLStore) UpdateLease(ctx context.Context, lease IntGateLease) error {
	  const q = `UPDATE int_gate_leases SET
		from_user_id=?, to_user_id=?, resolve_channel=?, to_hint=?, asset=?, amount=?,
		gate_fingerprint=?, gate_impl_id=?, gate_gen=?, halted=?, forbid_withdraw=?, state=?,
		ttl_seconds=?, issued_at=?, bound_at=?, sent_at=?, expired_at=?, revoked_at=?, revoke_reason=?,
		transfer_id=?, idempotency_key=?, send_idempotency_key=?, updated_at=?
		WHERE lease_id=?`
	   res, err := s.q(ctx).ExecContext(ctx, q,
		  lease.FromUserID, lease.ToUserID, lease.ResolveChannel, lease.ToHint, lease.Asset, lease.Amount,
		   lease.GateFingerprint, lease.GateImplID, lease.GateGen, boolToInt(lease.Halted),
		   boolToInt(lease.ForbidWithdraw), string(lease.State), lease.TTLSeconds, lease.IssuedAt,
		  nullTime(lease.BoundAt), nullTime(lease.SentAt), nullTime(lease.ExpiredAt),
		  nullTime(lease.RevokedAt), lease.RevokeReason, lease.TransferID, lease.IdempotencyKey,
		   lease.SendIdempotencyKey,
		   lease.UpdatedAt, lease.LeaseID,
	  )
	  if err != nil {
		  return WrapStore("update_lease", err)
	   }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrLeaseNotFound
	   }
	   return nil
   }

  func (s *SQLStore) ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]IntGateLease, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  const q = `SELECT lease_id, from_user_id, to_user_id, resolve_channel, to_hint, asset, amount,
		gate_fingerprint, gate_impl_id, gate_gen, halted, forbid_withdraw, state,
		ttl_seconds, issued_at, bound_at, sent_at, expired_at, revoked_at, revoke_reason,
		transfer_id, idempotency_key, send_idempotency_key, created_at, updated_at
		FROM int_gate_leases WHERE state=? LIMIT ?`
	   rows, err := s.q(ctx).QueryContext(ctx, q, string(state), limit)
	   if err != nil {
		  return nil, WrapStore("list_state", err)
	  }
	  defer rows.Close()
	  return scanLeases(rows)
   }

   func (s *SQLStore) ListExpiredGated(ctx context.Context, now time.Time, limit int) ([]IntGateLease, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	  const q = `SELECT lease_id, from_user_id, to_user_id, resolve_channel, to_hint, asset, amount,
		gate_fingerprint, gate_impl_id, gate_gen, halted, forbid_withdraw, state,
		ttl_seconds, issued_at, bound_at, sent_at, expired_at, revoked_at, revoke_reason,
		transfer_id, idempotency_key, send_idempotency_key, created_at, updated_at
		FROM int_gate_leases
		WHERE state IN ('gated','bound') AND DATE_ADD(issued_at, INTERVAL ttl_seconds SECOND) < ?
		LIMIT ?`
	  rows, err := s.q(ctx).QueryContext(ctx, q, now.UTC(), limit)
	  if err != nil {
		   return nil, WrapStore("list_expired", err)
	   }
	  defer rows.Close()
	  return scanLeases(rows)
   }

  func (s *SQLStore) ListUnconsumed(ctx context.Context, limit int) ([]IntGateLease, error) {
	  if limit <= 0 {
		   limit = 100
	  }
	  const q = `SELECT lease_id, from_user_id, to_user_id, resolve_channel, to_hint, asset, amount,
		gate_fingerprint, gate_impl_id, gate_gen, halted, forbid_withdraw, state,
		ttl_seconds, issued_at, bound_at, sent_at, expired_at, revoked_at, revoke_reason,
		transfer_id, idempotency_key, send_idempotency_key, created_at, updated_at
		FROM int_gate_leases WHERE state IN ('gated','bound') LIMIT ?`
	   rows, err := s.q(ctx).QueryContext(ctx, q, limit)
	   if err != nil {
		   return nil, WrapStore("list_unconsumed", err)
	   }
	   defer rows.Close()
	   return scanLeases(rows)
   }

  func (s *SQLStore) ListRevocableByGen(ctx context.Context, maxGen int64, limit int) ([]IntGateLease, error) {
	   if limit <= 0 {
		   limit = 200
	   }
	   const q = `SELECT lease_id, from_user_id, to_user_id, resolve_channel, to_hint, asset, amount, 
		gate_fingerprint, gate_impl_id, gate_gen, halted, forbid_withdraw, state,
		ttl_seconds, issued_at, bound_at, sent_at, expired_at, revoked_at, revoke_reason,
		transfer_id, idempotency_key, send_idempotency_key, created_at, updated_at
		FROM int_gate_leases WHERE state IN ('gated','bound') AND gate_gen < ? LIMIT ?`
	  rows, err := s.q(ctx).QueryContext(ctx, q, maxGen, limit)
	  if err != nil {
		   return nil, WrapStore("list_revocable", err)
	   }
	  defer rows.Close()
	  return scanLeases(rows)
   }

  func (s *SQLStore) SaveGen(ctx context.Context, gen GateGeneration) error {
	  const q = `INSERT INTO int_gate_generations (
		id, scope, gen, gate_impl_id, halted, forbid_withdraw, fingerprint, bump_reason, bumped_at, bumped_by
	) VALUES (?,?,?,?,?,?,?,?,?,?)
	ON DUPLICATE KEY UPDATE gen=VALUES(gen), gate_impl_id=VALUES(gate_impl_id),
		halted=VALUES(halted), forbid_withdraw=VALUES(forbid_withdraw),
		fingerprint=VALUES(fingerprint), bump_reason=VALUES(bump_reason),
		bumped_at=VALUES(bumped_at), bumped_by=VALUES(bumped_by)`
	   if gen.Scope == "" {
		  gen.Scope = ScopeGlobal
	   }
	   _,
		  err := s.q(ctx).ExecContext(ctx, q,
		  gen.ID, gen.Scope, gen.Gen, gen.GateImplID, boolToInt(gen.Halted),
		   boolToInt(gen.ForbidWithdraw), gen.Fingerprint, gen.BumpReason, gen.BumpedAt, gen.BumpedBy,
	   )
	   if err != nil {
		  return WrapStore("save_gen", err)
	   }
	   return nil
  }

  func (s *SQLStore) GetGen(ctx context.Context, scope string) (GateGeneration, error) {
	   if scope == "" {
		  scope = ScopeGlobal
	  }
	   const q = `SELECT id, scope, gen, gate_impl_id, halted, forbid_withdraw, fingerprint, bump_reason, bumped_at, bumped_by
		FROM int_gate_generations WHERE scope=?`
	  var g GateGeneration
	   var halted, forbid int
	   err := s.q(ctx).QueryRowContext(ctx, q, scope).Scan(
		  &g.ID, &g.Scope, &g.Gen, &g.GateImplID, &halted, &forbid, &g.Fingerprint,
		  &g.BumpReason, &g.BumpedAt, &g.BumpedBy,
	  )
	   if err == sql.ErrNoRows {
		  return GateGeneration{}, ErrGenNotFound
	   }
	   if err != nil {
		   return GateGeneration{}, WrapStore("get_gen", err)
	  }
	   g.Halted = halted != 0
	   g.ForbidWithdraw = forbid != 0
	   return g, nil
   }

   func (s *SQLStore) SaveProof(ctx context.Context, proof IntGateDriftProof) error {
	   const q = `INSERT INTO int_gate_drift_proofs (
		proof_id, kind, lease_id, from_user_id, to_user_id, asset, amount,
		bind_fingerprint, live_fingerprint, bind_gen, live_gen, bind_to_user, live_to_user,
		evidence, detected_at, exported_at, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.q(ctx).ExecContext(ctx, q,
		   proof.ProofID, string(proof.Kind), proof.LeaseID, proof.FromUserID, proof.ToUserID,
		   proof.Asset,
		   proof.Amount, proof.BindFingerprint, proof.LiveFingerprint,
		  proof.BindGen, proof.LiveGen, proof.BindToUser, proof.LiveToUser,
		  proof.Evidence, proof.DetectedAt, nullTime(proof.ExportedAt), proof.CreatedAt,
	   )
	  if err != nil {
		   return WrapStore("save_proof", err)
	   }
	  return nil
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (IntGateDriftProof, error) {
	  const q = `SELECT proof_id, kind, lease_id, from_user_id, to_user_id, asset, amount,
		bind_fingerprint, live_fingerprint, bind_gen, live_gen, bind_to_user, live_to_user,
		evidence, detected_at, exported_at, created_at
		FROM int_gate_drift_proofs WHERE proof_id=?`
	   var p IntGateDriftProof
	  var kind string
	   var exp sql.NullTime
	   err := s.q(ctx).QueryRowContext(ctx, q, proofID).Scan(
		  &p.ProofID, &kind, &p.LeaseID, &p.FromUserID, &p.ToUserID, &p.Asset, &p.Amount,
		  &p.BindFingerprint, &p.LiveFingerprint, &p.BindGen, &p.LiveGen, &p.BindToUser, &p.LiveToUser,
		  &p.Evidence, &p.DetectedAt, &exp, &p.CreatedAt,
	  )
	  if err == sql.ErrNoRows {
		   return IntGateDriftProof{}, ErrProofNotFound
	  }
	  if err != nil {
		   return IntGateDriftProof{}, WrapStore("get_proof", err)
	  }
	  p.Kind = ProofKind(kind)
	   if exp.Valid {
		  t := exp.Time.UTC()
		   p.ExportedAt = &t
	   }
	  return p, nil
  }

   func (s *SQLStore) ListProofs(ctx context.Context, filter ExportProofFilter) ([]IntGateDriftProof, error) {
	  limit := filter.Limit
	  if limit <= 0 {
		  limit = 100
	  }
	   q := `SELECT proof_id, kind, lease_id, from_user_id, to_user_id, asset, amount,
		bind_fingerprint, live_fingerprint, bind_gen, live_gen, bind_to_user, live_to_user,
		evidence, detected_at, exported_at, created_at FROM int_gate_drift_proofs WHERE 1=1`
	  args := make([]any, 0, 4)
	   if filter.Kind != "" {
		  q += ` AND kind=?`
		  args = append(args, string(filter.Kind))
	   }
	   if strings.TrimSpace(filter.LeaseID) != "" {
		  q += ` AND lease_id=?`
		   args = append(args, filter.LeaseID)
	  }
	   q += ` ORDER BY created_at DESC LIMIT ?`
	   args = append(args, limit)
	  rows, err := s.q(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, WrapStore("list_proofs", err)
	  }
	   defer rows.Close()
	   out := make([]IntGateDriftProof, 0)
	  for rows.Next() {
		  var p IntGateDriftProof
		  var kind string
		  var exp sql.NullTime
		  if err := rows.Scan(
			   &p.ProofID, &kind, &p.LeaseID, &p.FromUserID, &p.ToUserID, &p.Asset, &p.Amount,
			   &p.BindFingerprint, &p.LiveFingerprint, &p.BindGen, &p.LiveGen, &p.BindToUser, &p.LiveToUser,
			   &p.Evidence, &p.DetectedAt, &exp, &p.CreatedAt,
		   ); err != nil {
			   return nil, WrapStore("scan_proof", err)
		   }
		  p.Kind = ProofKind(kind)
		   if exp.Valid {
			  t := exp.Time.UTC()
			  p.ExportedAt = &t
		   }
		   out = append(out, p)
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) MarkProofsExported(ctx context.Context, ids []string, at time.Time) error {
	  if len(ids) == 0 {
		  return nil
	   }
	   for _, id := range ids {
		  _, err := s.q(ctx).ExecContext(ctx,
			  `UPDATE int_gate_drift_proofs SET exported_at=? WHERE proof_id=?`, at.UTC(), id)
		  if err != nil {
			  return WrapStore("mark_exported", err)
		   }
	  }
	  return nil
   }

  func (s *SQLStore) SaveAudit(ctx context.Context, row IntGateAudit) error {
	   const q = `INSERT INTO int_gate_audit (audit_id, lease_id, action, actor, detail, from_state, to_state, created_at) 
		VALUES (?,?,?,?,?,?,?,?)`
	  _,
		   err := s.q(ctx).ExecContext(ctx, q,
		  row.AuditID, row.LeaseID, string(row.Action), row.Actor, row.Detail, row.FromState, row.ToState, row.CreatedAt,
	   )
	  if err != nil {
		  return WrapStore("save_audit", err)
	  }
	  return nil
   }

  func (s *SQLStore) ListAuditByLease(ctx context.Context, leaseID string, limit int) ([]IntGateAudit, error) {
	   if limit <= 0 {
		  limit = 100
	   }
	   const q = `SELECT audit_id, lease_id, action, actor, detail, from_state, to_state, created_at
		FROM int_gate_audit WHERE lease_id=? ORDER BY created_at ASC LIMIT ?`
	   rows, err := s.q(ctx).QueryContext(ctx, q, leaseID, limit)
	   if err != nil {
		   return nil, WrapStore("list_audit", err)
	  }
	  defer rows.Close()
	   out := make([]IntGateAudit, 0)
	   for rows.Next() {
		   var a IntGateAudit
		  var action string
		   if err := rows.Scan(&a.AuditID, &a.LeaseID, &action, &a.Actor, &a.Detail, &a.FromState, &a.ToState, &a.CreatedAt); err != nil {
			   return nil, WrapStore("scan_audit", err)
		  }
		  a.Action = AuditAction(action)
		  out = append(out, a)
	  }
	   return out, rows.Err()
  }

type rowScanner interface {
	Scan(dest ...any) error
}

  func scanLease(row rowScanner) (IntGateLease, error) {
	   var l IntGateLease
	   var state string
	  var halted, forbid int
	  var bound, sent, exp, rev sql.NullTime
	  err := row.Scan(
		  &l.LeaseID, &l.FromUserID, &l.ToUserID, &l.ResolveChannel, &l.ToHint, &l.Asset, &l.Amount,
		   &l.GateFingerprint, &l.GateImplID, &l.GateGen, &halted, &forbid, &state,
		  &l.TTLSeconds,
		  &l.IssuedAt, &bound, &sent, &exp, &rev, &l.RevokeReason,
		   &l.TransferID, &l.IdempotencyKey, &l.SendIdempotencyKey, &l.CreatedAt, &l.UpdatedAt,
	  )
	  if err != nil {
		  return IntGateLease{}, err
	   }
	  l.State = LeaseState(state)
	   l.Halted = halted != 0
	  l.ForbidWithdraw = forbid != 0
	   l.BoundAt = nullToPtr(bound)
	   l.SentAt = nullToPtr(sent)
	  l.ExpiredAt = nullToPtr(exp)
	   l.RevokedAt = nullToPtr(rev)
	  return l, nil
   }

  func scanLeases(rows *sql.Rows) ([]IntGateLease, error) {
	  out := make([]IntGateLease, 0)
	   for rows.Next() {
		  l, err := scanLease(rows)
		  if err != nil {
			   return nil, WrapStore("scan_lease", err)
		  }
		   out = append(out, l)
	   }
	  return out, rows.Err()
   }

   func boolToInt(v bool) int {
	   if v {
		  return 1
	   }
	   return 0
   }

  func nullTime(t *time.Time) any {
	  if t == nil {
		  return nil
	  }
	  return t.UTC()
  }

  func nullToPtr(n sql.NullTime) *time.Time {
	  if !n.Valid {
		   return nil
	  }
	  t := n.Time.UTC()
	  return &t
  }
