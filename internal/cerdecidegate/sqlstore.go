package cerdecidegate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore 仓。

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 包装 *sql.DB。
  func NewSQLStore(db *sql.DB) *SQLStore {
	   return &SQLStore{db: db}
   }

const qInsertLease = `
INSERT IGNORE INTO cer_decide_leases (
  lease_id, batch_id, asset, chain, amount, dest, dest_risk, side, purpose,
  policy_id, threshold_n, shares_n, bind_mode, required_flag, state, ttl_sec,
  idempotency_key, actor, generation, flip_epoch, exec_ref,
  issued_at, bound_at, consumed_at, expired_at, revoked_at, revoke_why,
  pack_digest, line_digest, snap_digest, fp_digest, force_cold,
  created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

  func (s *SQLStore) SaveLease(ctx context.Context, l DecideLease) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	  _,
		  err := s.db.ExecContext(ctx, qInsertLease,
		   l.LeaseID, l.BatchID, l.Asset, l.Chain, l.Amount, l.Dest, l.DestRisk, l.Side, l.Purpose,
		   l.PolicyID, l.Threshold, l.Shares, l.BindMode, boolInt(l.Required), string(l.State), l.TTLSec,
		  l.Idem,
		   l.Actor, l.Generation, l.FlipEpoch, l.ExecRef,
		  l.IssuedAt, nullTime(l.BoundAt), nullTime(l.ConsumedAt), nullTime(l.ExpiredAt), nullTime(l.RevokedAt), l.RevokeWhy,
		   l.Fingerprint.PackDigest, l.Fingerprint.LineDigest, l.Fingerprint.SnapDigest, l.Fingerprint.Digest, boolInt(l.Fingerprint.ForceCold),
		  l.CreatedAt, l.UpdatedAt,
	  )
	  if err != nil {
		   return ErrStore
	  }
	   return nil
   }

const qSelectLease = `
SELECT lease_id, batch_id, asset, chain, amount, dest, dest_risk, side, purpose,
  policy_id, threshold_n, shares_n, bind_mode, required_flag, state, ttl_sec,
  idempotency_key, actor, generation, flip_epoch, exec_ref,
  issued_at, bound_at, consumed_at, expired_at, revoked_at, revoke_why,
  pack_digest, line_digest, snap_digest, fp_digest, force_cold,
  created_at, updated_at
FROM cer_decide_leases`

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (DecideLease, error) {
	  if s == nil || s.db == nil {
		  return DecideLease{}, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, qSelectLease+" WHERE lease_id=?", strings.TrimSpace(leaseID))
	   return scanLease(row)
  }

   func (s *SQLStore) GetLeaseByIdem(ctx context.Context, idem string) (DecideLease, error) {
	   if s == nil || s.db == nil {
		  return DecideLease{}, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, qSelectLease+" WHERE idempotency_key=?", strings.TrimSpace(idem))
	  return scanLease(row)
   }

   func (s *SQLStore) ListOpenByScope(ctx context.Context, scope string) ([]DecideLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  q := qSelectLease + ` WHERE state IN ('issued','bound')`
	   args := []any{}
	   if strings.HasPrefix(scope, "batch:") {
		  q += ` AND batch_id=?`
		  args = append(args, strings.TrimPrefix(scope, "batch:"))
	  } else if strings.HasPrefix(scope, "policy:") {
		  q += ` AND policy_id=?`
		  args = append(args, strings.TrimPrefix(scope, "policy:"))
	   } else if strings.HasPrefix(scope, "asset:") {
		  parts := strings.SplitN(strings.TrimPrefix(scope, "asset:"), ":", 2)
		   if len(parts) == 2 {
			  q += ` AND asset=? AND chain=?`
			   args = append(args, parts[0], parts[1])
		  }
	  }
	   return s.queryLeases(ctx, q, args...)
  }

   func (s *SQLStore) ListOpenByPolicy(ctx context.Context, policyID string) ([]DecideLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  return s.queryLeases(ctx, qSelectLease+` WHERE state IN ('issued','bound') AND policy_id=?`, strings.TrimSpace(policyID))
  }

   func (s *SQLStore) ListOpenByBatch(ctx context.Context, batchID string) ([]DecideLease, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   return s.queryLeases(ctx, qSelectLease+` WHERE state IN ('issued','bound') AND batch_id=?`, strings.TrimSpace(batchID))
   }

  func (s *SQLStore) ListIssuedOrBound(ctx context.Context, now time.Time, limit int) ([]DecideLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 64
	  }
	   _ = now
	   return s.queryLeases(ctx, qSelectLease+` WHERE state IN ('issued','bound') ORDER BY issued_at ASC LIMIT ?`, limit)
   }

   func (s *SQLStore) CASState(ctx context.Context, leaseID string, from, to LeaseState, patch DecideLease) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	  if err := NextError(from, to); err != nil && !Allow(from, to) {
		   return err
	   }
	   res, err := s.db.ExecContext(ctx, `
UPDATE cer_decide_leases SET state=?, bound_at=COALESCE(?, bound_at), consumed_at=COALESCE(?, consumed_at),
  expired_at=COALESCE(?, expired_at), revoked_at=COALESCE(?, revoked_at),
  revoke_why=CASE WHEN ?<>'' THEN ? ELSE revoke_why END,
  exec_ref=CASE WHEN ?<>'' THEN ? ELSE exec_ref END,
  updated_at=?
WHERE lease_id=? AND state=?`,
		   string(to), nullTime(patch.BoundAt), nullTime(patch.ConsumedAt),
		  nullTime(patch.ExpiredAt), nullTime(patch.RevokedAt),
		   patch.RevokeWhy, patch.RevokeWhy, patch.ExecRef, patch.ExecRef,
		   patch.UpdatedAt, strings.TrimSpace(leaseID), string(from),
	   )
	   if err != nil {
		   return ErrStore
	   }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	   }
	   return nil
  }

  func (s *SQLStore) SaveProof(ctx context.Context, p CerDriftProof) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, `
INSERT IGNORE INTO cer_drift_proofs (
  proof_id, lease_id, batch_id, kind, quoted_policy, live_policy,
  quoted_thresh, live_thresh, quoted_shares, live_shares,
  quoted_risk, live_risk, quoted_force, live_force,
  quoted_gen, live_gen, quoted_digest, live_digest,
  detail, actor, at_ts, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ProofID, p.LeaseID, p.BatchID, string(p.Kind), p.QuotedPolicy, p.LivePolicy,
		   p.QuotedThresh, p.LiveThresh, p.QuotedShares, p.LiveShares,
		  p.QuotedRisk, p.LiveRisk, boolInt(p.QuotedForce), boolInt(p.LiveForce),
		  p.QuotedGen, p.LiveGen, p.QuotedDigest, p.LiveDigest,
		   p.Detail, p.Actor, p.At, p.CreatedAt,
	   )
	   if err != nil {
		   return ErrStore
	  }
	  return nil
   }

   func (s *SQLStore) GetProof(ctx context.Context, proofID string) (CerDriftProof, error) {
	  if s == nil || s.db == nil {
		   return CerDriftProof{}, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, qSelectProof+" WHERE proof_id=?", strings.TrimSpace(proofID))
	  return scanProof(row)
  }

const qSelectProof = `
SELECT proof_id, lease_id, batch_id, kind, quoted_policy, live_policy,
  quoted_thresh, live_thresh, quoted_shares, live_shares,
  quoted_risk, live_risk, quoted_force, live_force,
  quoted_gen, live_gen, quoted_digest, live_digest,
  detail, actor, at_ts, created_at
FROM cer_drift_proofs`

   func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string) ([]CerDriftProof, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  return s.queryProofs(ctx, qSelectProof+" WHERE lease_id=?", strings.TrimSpace(leaseID))
   }

  func (s *SQLStore) ListProofsByBatch(ctx context.Context, batchID string, limit int) ([]CerDriftProof, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 64
	   }
	  return s.queryProofs(ctx, qSelectProof+" WHERE batch_id=? ORDER BY at_ts DESC LIMIT ?", strings.TrimSpace(batchID), limit)
   }

   func (s *SQLStore) ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]CerDriftProof, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 64
	   }
	   return s.queryProofs(ctx, qSelectProof+" WHERE kind=? ORDER BY at_ts DESC LIMIT ?", string(kind), limit)
  }

   func (s *SQLStore) AppendAudit(ctx context.Context, a DecideAudit) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO cer_decide_audit (audit_id, lease_id, batch_id, action, from_state, to_state, actor, detail, at_ts)
VALUES (?,?,?,?,?,?,?,?,?)`,
		  a.AuditID, a.LeaseID, a.BatchID, a.Action, a.From, a.To, a.Actor, a.Detail, a.At)
	   if err != nil {
		  return ErrStore
	  }
	   return nil
   }

  func (s *SQLStore) ListAudit(ctx context.Context, leaseID string) ([]DecideAudit, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   rows, err := s.db.QueryContext(ctx, `
SELECT audit_id, lease_id, batch_id, action, from_state, to_state, actor, detail, at_ts
FROM cer_decide_audit WHERE lease_id=? ORDER BY at_ts ASC`, strings.TrimSpace(leaseID))
	  if err != nil {
		  return nil, ErrStore
	   }
	  defer rows.Close()
	   out := make([]DecideAudit, 0)
	  for rows.Next() {
		  var a DecideAudit
		  if err := rows.Scan(&a.AuditID, &a.LeaseID, &a.BatchID, &a.Action, &a.From, &a.To, &a.Actor, &a.Detail, &a.At); err != nil {
			  return nil, ErrStore
		  }
		   out = append(out, a)
	  }
	  return out, rows.Err()
   }

  func (s *SQLStore) SaveFlip(ctx context.Context, e FlipEvent) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, `
INSERT IGNORE INTO cer_decide_flip_events (
  event_id, scope_key, kind, policy_id, old_force, new_force,
  old_thresh, new_thresh, old_shares, new_shares, old_risk, new_risk,
  epoch, at_ts, done_flag
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  e.EventID, e.Scope, string(e.Kind), e.PolicyID, boolInt(e.OldForce), boolInt(e.NewForce),
		   e.OldThresh, e.NewThresh, e.OldShares, e.NewShares, e.OldRisk, e.NewRisk,
		   e.Epoch, e.At, boolInt(e.Done),
	  )
	  if err != nil {
		  return ErrStore
	   }
	   return nil
  }

   func (s *SQLStore) ListPendingFlips(ctx context.Context, limit int) ([]FlipEvent, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 64
	   }
	  rows, err := s.db.QueryContext(ctx, `
SELECT event_id, scope_key, kind, policy_id, old_force, new_force,
  old_thresh, new_thresh, old_shares, new_shares, old_risk, new_risk,
  epoch, at_ts, done_flag
FROM cer_decide_flip_events WHERE done_flag=0 ORDER BY at_ts ASC LIMIT ?`, limit)
	  if err != nil {
		   return nil, ErrStore
	  }
	  defer rows.Close()
	  out := make([]FlipEvent, 0)
	  for rows.Next() {
		   var e FlipEvent
		  var kind string
		   var of, nf, done int
		  if err := rows.Scan(&e.EventID, &e.Scope, &kind, &e.PolicyID, &of, &nf,
			   &e.OldThresh, &e.NewThresh, &e.OldShares, &e.NewShares, &e.OldRisk, &e.NewRisk,
			   &e.Epoch, &e.At, &done); err != nil {
			   return nil, ErrStore
		  }
		   e.Kind = FlipKind(kind)
		  e.OldForce, e.NewForce, e.Done = of == 1, nf == 1, done == 1
		  out = append(out, e)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) MarkFlipDone(ctx context.Context, eventID string) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	   _, err := s.db.ExecContext(ctx, `UPDATE cer_decide_flip_events SET done_flag=1 WHERE event_id=?`, strings.TrimSpace(eventID))
	  if err != nil {
		   return ErrStore
	   }
	   return nil
  }

   func (s *SQLStore) NextGeneration(ctx context.Context, scope string) (int64, error) {
	  if s == nil || s.db == nil {
		   return 0, ErrUnavailable
	  }
	   scope = strings.TrimSpace(scope)
	  if scope == "" {
		  scope = "*"
	   }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO cer_decide_generations (scope_key, epoch, updated_at)
VALUES (?, 1, CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE epoch=epoch+1, updated_at=CURRENT_TIMESTAMP(3)`, scope)
	   if err != nil {
		  return 0, ErrStore
	  }
	  return s.CurrentGeneration(ctx, scope)
   }

   func (s *SQLStore) CurrentGeneration(ctx context.Context, scope string) (int64, error) {
	  if s == nil || s.db == nil {
		   return 0, ErrUnavailable
	   }
	   scope = strings.TrimSpace(scope)
	   if scope == "" {
		  scope = "*"
	  }
	  var n int64
	   err := s.db.QueryRowContext(ctx, `SELECT epoch FROM cer_decide_generations WHERE scope_key=?`, scope).Scan(&n)
	   if err == sql.ErrNoRows {
		  _, _ = s.db.ExecContext(ctx, `INSERT IGNORE INTO cer_decide_generations (scope_key, epoch, updated_at) VALUES (?,1,CURRENT_TIMESTAMP(3))`, scope)
		  return 1, nil
	  }
	   if err != nil {
		   return 0, ErrStore
	  }
	   if n == 0 {
		   n = 1
	  }
	   return n, nil
   }

   func (s *SQLStore) queryLeases(ctx context.Context, q string, args ...any) ([]DecideLease, error) {
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, ErrStore
	   }
	  defer rows.Close()
	  out := make([]DecideLease, 0)
	  for rows.Next() {
		  l, err := scanLease(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, l)
	  }
	   return out, rows.Err()
   }

   func (s *SQLStore) queryProofs(ctx context.Context, q string, args ...any) ([]CerDriftProof, error) {
	   rows,
		   err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, ErrStore
	  }
	  defer rows.Close()
	  out := make([]CerDriftProof, 0)
	  for rows.Next() {
		   p, err := scanProof(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, p)
	  }
	   return out, rows.Err()
  }

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanLease(row rowScanner) (DecideLease, error) {
	  var l DecideLease
	  var state string
	   var req, force int
	   var bound, cons, exp, rev sql.NullTime
	  err := row.Scan(
		  &l.LeaseID, &l.BatchID, &l.Asset, &l.Chain, &l.Amount, &l.Dest, &l.DestRisk, &l.Side, &l.Purpose,
		   &l.PolicyID,
		   &l.Threshold, &l.Shares, &l.BindMode, &req, &state, &l.TTLSec,
		   &l.Idem, &l.Actor, &l.Generation, &l.FlipEpoch, &l.ExecRef,
		   &l.IssuedAt, &bound, &cons, &exp, &rev, &l.RevokeWhy,
		   &l.Fingerprint.PackDigest, &l.Fingerprint.LineDigest, &l.Fingerprint.SnapDigest, &l.Fingerprint.Digest, &force,
		  &l.CreatedAt, &l.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		   return DecideLease{}, ErrLeaseNotFound
	  }
	  if err != nil {
		   return DecideLease{}, ErrStore
	  }
	   l.Required = req == 1
	  l.State = LeaseState(state)
	   l.Fingerprint.PolicyID = l.PolicyID
	   l.Fingerprint.Threshold = l.Threshold
	   l.Fingerprint.Shares = l.Shares
	  l.Fingerprint.BindMode = l.BindMode
	   l.Fingerprint.DestRisk = l.DestRisk
	   l.Fingerprint.Required = l.Required
	  l.Fingerprint.ForceCold = force == 1
	   l.Fingerprint.Generation = l.Generation
	  if bound.Valid {
		  l.BoundAt = bound.Time
	  }
	  if cons.Valid {
		   l.ConsumedAt = cons.Time
	  }
	   if exp.Valid {
		   l.ExpiredAt = exp.Time
	   }
	  if rev.Valid {
		  l.RevokedAt = rev.Time
	   }
	   return l, nil
   }

  func scanProof(row rowScanner) (CerDriftProof, error) {
	   var p CerDriftProof
	  var kind string
	  var qf, lf int
	  err := row.Scan(
		   &p.ProofID, &p.LeaseID, &p.BatchID, &kind, &p.QuotedPolicy, &p.LivePolicy,
		   &p.QuotedThresh, &p.LiveThresh, &p.QuotedShares, &p.LiveShares,
		   &p.QuotedRisk, &p.LiveRisk, &qf, &lf,
		  &p.QuotedGen, &p.LiveGen, &p.QuotedDigest, &p.LiveDigest,
		  &p.Detail, &p.Actor, &p.At, &p.CreatedAt,
	  )
	   if err == sql.ErrNoRows {
		  return CerDriftProof{}, ErrProofNotFound
	  }
	   if err != nil {
		  return CerDriftProof{}, ErrStore
	  }
	   p.Kind = ProofKind(kind)
	   p.QuotedForce, p.LiveForce = qf == 1, lf == 1
	   return p, nil
   }

   func boolInt(v bool) int {
	  if v {
		   return 1
	  }
	  return 0
   }

  func nullTime(t time.Time) any {
	  if t.IsZero() {
		   return nil
	  }
	  return t
  }
