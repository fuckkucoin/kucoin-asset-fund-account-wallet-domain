// @FileName: sqlstore.go
// @Summary: egressleasegate 包内 sqlstore 实现。
// @Author: Cason
// @Created: 2021.04.25

package egressleasegate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

// SQLStore 将出金租约写入 elg_egress_leases、elg_egress_drift_proofs、
// elg_egress_lease_audit 与 elg_egress_generations。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 绑定数据库连接。连接为空时所有调用返回不可用。

   func NewSQLStore(db *sql.DB) *SQLStore {
	   return &SQLStore{db: db}
  }

   func (s *SQLStore) ready() error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	  return nil
   }

const leaseCols = `id, chain, asset, native, amount, decision_at_issue, breach_level, flags_fingerprint, tank_digest, halt_generation,
source, state, idem_key, broadcast_ref, batch_id, actor, issued_at, expires_at, bound_at, consumed_at, revoked_at,
revoke_reason, drift_proof_id, created_at, updated_at`

   func (s *SQLStore) InsertLease(ctx context.Context, l *EgressLease) error {
	   if err := s.ready(); err != nil {
		  return err
	  }
	   if l == nil || l.ID == "" {
		  return ErrUnavailable
	  }
	   var idem any
	   if strings.TrimSpace(l.IdemKey) != "" {
		  idem = l.IdemKey
	   }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO elg_egress_leases (`+leaseCols+`)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   l.ID, l.Chain, l.Asset, l.Native, l.Amount, string(l.DecisionAtIssue), l.BreachLevel, l.FlagsFingerprint, l.TankDigest, l.HaltGeneration,
		  l.Source, string(l.State), idem, l.BroadcastRef, l.BatchID, l.Actor, l.IssuedAt, l.ExpiresAt,
		   nullTime(l.BoundAt),
		   nullTime(l.ConsumedAt), nullTime(l.RevokedAt),
		  l.RevokeReason,
		   l.DriftProofID, l.CreatedAt, l.UpdatedAt,
	  )
	  if isDup(err) {
		  return ErrDuplicate
	  }
	  return err
  }

   func (s *SQLStore) GetLease(ctx context.Context, id string) (*EgressLease, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	   row := s.db.QueryRowContext(ctx, `SELECT `+leaseCols+` FROM elg_egress_leases WHERE id=?`, id)
	   return scanLease(row)
  }

   func (s *SQLStore) GetLeaseByIdem(ctx context.Context, idemKey string) (*EgressLease, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	  if strings.TrimSpace(idemKey) == "" {
		   return nil, ErrNotFound
	  }
	  row := s.db.QueryRowContext(ctx, `SELECT `+leaseCols+` FROM elg_egress_leases WHERE idem_key=?`, idemKey)
	   return scanLease(row)
   }

  func (s *SQLStore) UpdateLeaseCAS(ctx context.Context, l *EgressLease, from LeaseState) error {
	  if err := s.ready(); err != nil {
		   return err
	  }
	   if l == nil || l.ID == "" {
		  return ErrUnavailable
	   }
	  res, err := s.db.ExecContext(ctx, `
UPDATE elg_egress_leases
SET chain=?, asset=?, native=?, amount=?, decision_at_issue=?, breach_level=?, flags_fingerprint=?, tank_digest=?, halt_generation=?,
    source=?, state=?, broadcast_ref=?, batch_id=?, actor=?, issued_at=?, expires_at=?, bound_at=?, consumed_at=?, revoked_at=?,
    revoke_reason=?, drift_proof_id=?, updated_at=?
WHERE id=? AND state=?`,
		   l.Chain,
		   l.Asset, l.Native, l.Amount, string(l.DecisionAtIssue), l.BreachLevel, l.FlagsFingerprint, l.TankDigest, l.HaltGeneration,
		  l.Source, string(l.State), l.BroadcastRef, l.BatchID, l.Actor, l.IssuedAt, l.ExpiresAt,
		  nullTime(l.BoundAt), nullTime(l.ConsumedAt), nullTime(l.RevokedAt),
		  l.RevokeReason, l.DriftProofID, l.UpdatedAt, l.ID, string(from),
	  )
	  if err != nil {
		  return err
	   }
	  n, err := res.RowsAffected()
	  if err != nil {
		  return err
	   }
	   if n == 1 {
		  return nil
	   }
	   if _, gerr := s.GetLease(ctx, l.ID); gerr != nil {
		  return gerr
	   }
	  return ErrInvalidTransit
  }

   func (s *SQLStore) ListOpen(ctx context.Context, chain, asset string, limit int) ([]*EgressLease, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	   if limit <= 0 {
		  limit = 200
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT `+leaseCols+` FROM elg_egress_leases
WHERE state IN ('issued','bound') AND (?='' OR chain=?) AND (?='' OR asset=?)
ORDER BY issued_at, id LIMIT ?`, chain, chain, asset, asset, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  return scanLeases(rows)
  }

  func (s *SQLStore) ListExpired(ctx context.Context, before time.Time, limit int) ([]*EgressLease, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		   limit = 200
	  }
	   rows, err := s.db.QueryContext(ctx, `
SELECT `+leaseCols+` FROM elg_egress_leases
WHERE state IN ('issued','bound') AND expires_at < ?
ORDER BY expires_at LIMIT ?`, before, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanLeases(rows)
  }

const proofCols = `id, lease_id, chain, asset, drift_type, old_flags_fingerprint, new_flags_fingerprint, old_breach, new_breach,
old_tank_digest, new_tank_digest, old_halt_generation, new_halt_generation, broadcast_ref, state, detail, idem_key,
opened_at, proven_at, closed_at, updated_at`

   func (s *SQLStore) InsertProof(ctx context.Context, p *EgressDriftProof) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	  if p == nil || p.ID == "" {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO elg_egress_drift_proofs (`+proofCols+`)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ID, p.LeaseID, p.Chain, p.Asset, string(p.DriftType), p.OldFlagsFingerprint, p.NewFlagsFingerprint, p.OldBreach, p.NewBreach,
		  p.OldTankDigest,
		  p.NewTankDigest, p.OldHaltGeneration, p.NewHaltGeneration, p.BroadcastRef, string(p.State), p.Detail, p.IdemKey,
		  p.OpenedAt, nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.UpdatedAt,
	   )
	  if isDup(err) {
		   return ErrDuplicate
	  }
	  return err
   }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (*EgressDriftProof, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	   }
	  row := s.db.QueryRowContext(ctx, `SELECT `+proofCols+` FROM elg_egress_drift_proofs WHERE id=?`, id)
	   return scanProof(row)
   }

  func (s *SQLStore) GetProofByIdem(ctx context.Context, idemKey string) (*EgressDriftProof, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	   if idemKey == "" {
		   return nil, ErrNotFound
	  }
	   row := s.db.QueryRowContext(ctx, `SELECT `+proofCols+` FROM elg_egress_drift_proofs WHERE idem_key=?`, idemKey)
	   return scanProof(row)
   }

   func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]*EgressDriftProof, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		   limit = 200
	   }
	   rows, err := s.db.QueryContext(ctx, `SELECT `+proofCols+` FROM elg_egress_drift_proofs WHERE lease_id=? ORDER BY created_at LIMIT ?`, leaseID, limit)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  return scanProofs(rows)
  }

  func (s *SQLStore) ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*EgressDriftProof, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	   if limit <= 0 {
		  limit = 200
	   }
	  q := `SELECT ` + proofCols + ` FROM elg_egress_drift_proofs`
	  args := []any{}
	  if state != "" {
		   q += ` WHERE state=?`
		  args = append(args, string(state))
	  }
	   q += ` ORDER BY created_at LIMIT ?`
	   args = append(args, limit)
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanProofs(rows)
  }

   func (s *SQLStore) UpdateProof(ctx context.Context, p *EgressDriftProof) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	  if p == nil || p.ID == "" {
		   return ErrUnavailable
	  }
	   res, err := s.db.ExecContext(ctx, `
UPDATE elg_egress_drift_proofs
SET state=?, detail=?, proven_at=?, closed_at=?, updated_at=?
WHERE id=?`, string(p.State), p.Detail, nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.UpdatedAt, p.ID)
	   if err != nil {
		   return err
	   }
	  n, err := res.RowsAffected()
	   if err != nil {
		  return err
	  }
	   if n == 0 {
		   return ErrNotFound
	  }
	   return nil
  }

   func (s *SQLStore) AppendAudit(ctx context.Context, row *LeaseAudit) error {
	   if err := s.ready(); err != nil {
		  return err
	   }
	  if row == nil || row.ID == "" {
		  return ErrUnavailable
	   }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO elg_egress_lease_audit (id, lease_id, from_state, to_state, action, actor, detail, at)
VALUES (?,?,?,?,?,?,?,?)`, row.ID, row.LeaseID, row.FromState, row.ToState, row.Action, row.Actor, row.Detail, row.At)
	   if isDup(err) {
		   return ErrDuplicate
	  }
	  return err
   }

   func (s *SQLStore) ListAudit(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	  if limit <= 0 {
		   limit = 200
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT id, lease_id, from_state, to_state, action, actor, detail, at
FROM elg_egress_lease_audit WHERE (?='' OR lease_id=?) ORDER BY at, id LIMIT ?`, leaseID, leaseID, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]*LeaseAudit, 0)
	   for rows.Next() {
		  var a LeaseAudit
		   if err := rows.Scan(&a.ID, &a.LeaseID, &a.FromState, &a.ToState, &a.Action, &a.Actor, &a.Detail, &a.At); err != nil {
			  return nil, err
		  }
		  out = append(out, &a)
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) GetGeneration(ctx context.Context, scopeKey string) (*EgressGeneration, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	  }
	   row := s.db.QueryRowContext(ctx, `
SELECT scope_key, chain, asset, generation, flags_fp, breach_level, tank_digest, decision, reason, updated_at
FROM elg_egress_generation_scopes WHERE scope_key=?`, scopeKey)
	   var g EgressGeneration
	   if err := row.Scan(&g.ScopeKey, &g.Chain, &g.Asset, &g.Generation, &g.FlagsFP, &g.Breach, &g.TankDigest, &g.Decision, &g.Reason, &g.UpdatedAt); err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrNotFound
		  }
		  return nil, err
	  }
	   return &g, nil
   }

   func (s *SQLStore) PutGeneration(ctx context.Context, g *EgressGeneration) error {
	   if err := s.ready(); err != nil {
		  return err
	   }
	   if g == nil || g.ScopeKey == "" {
		   return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO elg_egress_generation_scopes
(scope_key, chain, asset, generation, flags_fp, breach_level, tank_digest, decision, reason, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
generation=VALUES(generation), flags_fp=VALUES(flags_fp), breach_level=VALUES(breach_level),
tank_digest=VALUES(tank_digest), decision=VALUES(decision), reason=VALUES(reason), updated_at=VALUES(updated_at)`,
		   g.ScopeKey, g.Chain, g.Asset, g.Generation, g.FlagsFP, g.Breach, g.TankDigest, g.Decision, g.Reason, g.UpdatedAt)
	   return err
   }

type rower interface {
	Scan(dest ...any) error
}

  func scanLease(row rower) (*EgressLease, error) {
	   var l EgressLease
	  var decision, state, idem, source sql.NullString
	   var bound, consumed, revoked sql.NullTime
	   err := row.Scan(
		  &l.ID, &l.Chain, &l.Asset, &l.Native, &l.Amount, &decision, &l.BreachLevel, &l.FlagsFingerprint, &l.TankDigest, &l.HaltGeneration,
		   &source, &state, &idem, &l.BroadcastRef, &l.BatchID, &l.Actor, &l.IssuedAt, &l.ExpiresAt, &bound, &consumed, &revoked,
		  &l.RevokeReason, &l.DriftProofID, &l.CreatedAt, &l.UpdatedAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	   }
	  if err != nil {
		  return nil, err
	   }
	   l.State = LeaseState(state.String)
	  l.Decision = decision.String
	   l.DecisionAtIssue = floatdesk.InstantDecision(decision.String)
	   l.FlagsFP = l.FlagsFingerprint
	  l.Generation = l.HaltGeneration
	  if source.Valid {
		   l.Source = source.String
	  }
	  if idem.Valid {
		  l.IdemKey = idem.String
	  }
	  l.BoundAt = timePtr(bound)
	   l.ConsumedAt = timePtr(consumed)
	   l.RevokedAt = timePtr(revoked)
	  return &l, nil
  }

  func scanLeases(rows *sql.Rows) ([]*EgressLease, error) {
	   out := make([]*EgressLease, 0)
	  for rows.Next() {
		   l, err := scanLease(rows)
		  if err != nil {
			   return nil, err
		  }
		   out = append(out, l)
	  }
	   return out, rows.Err()
   }

  func scanProof(row rower) (*EgressDriftProof, error) {
	  var p EgressDriftProof
	  var dt, state string
	  var proven, closed sql.NullTime
	  err := row.Scan(
		  &p.ID, &p.LeaseID, &p.Chain, &p.Asset, &dt, &p.OldFlagsFingerprint, &p.NewFlagsFingerprint, &p.OldBreach, &p.NewBreach,
		   &p.OldTankDigest, &p.NewTankDigest, &p.OldHaltGeneration, &p.NewHaltGeneration, &p.BroadcastRef, &state, &p.Detail, &p.IdemKey,
		  &p.OpenedAt, &proven, &closed, &p.UpdatedAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	   }
	  if err != nil {
		   return nil, err
	   }
	   p.DriftType = DriftType(dt)
	  p.State = ProofState(state)
	   p.ProvenAt = timePtr(proven)
	  p.ClosedAt = timePtr(closed)
	  return &p, nil
   }

  func scanProofs(rows *sql.Rows) ([]*EgressDriftProof, error) {
	   out := make([]*EgressDriftProof, 0)
	  for rows.Next() {
		   p, err := scanProof(rows)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, p)
	   }
	   return out, rows.Err()
   }

  func nullTime(t *time.Time) any {
	   if t == nil || t.IsZero() {
		  return nil
	  }
	  return *t
  }

  func timePtr(n sql.NullTime) *time.Time {
	  if !n.Valid {
		  return nil
	   }
	  t := n.Time
	  return &t
  }

   func isDup(err error) bool {
	  if err == nil {
		   return false
	   }
	   msg := strings.ToLower(err.Error())
	  return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
   }

  func (s *SQLStore) GetLeaseByID(ctx context.Context, id string) (*EgressLease, error) {
	  return s.GetLease(ctx, id)
  }

  func (s *SQLStore) UpdateLease(ctx context.Context, l *EgressLease) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	   if l == nil || l.ID == "" {
		  return ErrUnavailable
	  }
	   res, err := s.db.ExecContext(ctx, `
UPDATE elg_egress_leases
SET chain=?, asset=?, native=?, amount=?, decision_at_issue=?, breach_level=?, flags_fingerprint=?, tank_digest=?, halt_generation=?,
    source=?, state=?, broadcast_ref=?, batch_id=?, actor=?, issued_at=?, expires_at=?, bound_at=?, consumed_at=?, revoked_at=?,
    revoke_reason=?, drift_proof_id=?, updated_at=?
WHERE id=?`,
		  l.Chain,
		  l.Asset, l.Native, l.Amount, string(l.DecisionAtIssue), l.BreachLevel, l.FlagsFingerprint, l.TankDigest, l.HaltGeneration,
		  l.Source, string(l.State), l.BroadcastRef, l.BatchID, l.Actor, l.IssuedAt, l.ExpiresAt,
		   nullTime(l.BoundAt), nullTime(l.ConsumedAt), nullTime(l.RevokedAt),
		   l.RevokeReason, l.DriftProofID, l.UpdatedAt, l.ID,
	  )
	   if err != nil {
		  return err
	   }
	  n, err := res.RowsAffected()
	   if err != nil {
		   return err
	   }
	  if n == 0 {
		  return ErrNotFound
	   }
	  return nil
   }

  func (s *SQLStore) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*EgressLease, error) {
	  return s.ListExpired(ctx,
		  before, limit)
  }

  func (s *SQLStore) ListActiveLeases(ctx context.Context, limit int) ([]*EgressLease, error) {
	   return s.ListOpen(ctx, "", "", limit)
  }

   func (s *SQLStore) ListActiveByScope(ctx context.Context, chain, asset string, limit int) ([]*EgressLease, error) {
	  return s.ListOpen(ctx, chain, asset, limit)
  }

   func (s *SQLStore) GetOpenProofByLease(ctx context.Context, leaseID string) (*EgressDriftProof, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	  row := s.db.QueryRowContext(ctx, `SELECT `+proofCols+` FROM elg_egress_drift_proofs WHERE lease_id=? AND state='open' ORDER BY created_at DESC LIMIT 1`, leaseID)
	  return scanProof(row)
   }

  func (s *SQLStore) GetProofByID(ctx context.Context, id string) (*EgressDriftProof, error) {
	   return s.GetProof(ctx, id)
  }

   func (s *SQLStore) InsertAudit(ctx context.Context, a *LeaseAudit) error {
	   if a != nil {
		   if a.Action == "" {
			   a.Action = a.Note
		  }
		   if a.Detail == "" {
			  a.Detail = a.Note
		   }
		   if a.At.IsZero() {
			  a.At = a.CreatedAt
		  }
	  }
	   return s.AppendAudit(ctx, a)
   }

   func (s *SQLStore) ListAuditByLease(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error) {
	   return s.ListAudit(ctx, leaseID, limit)
   }

  func (s *SQLStore) InsertGeneration(ctx context.Context, g *EgressGeneration) error {
	  if g == nil {
		   return ErrUnavailable
	  }
	   if g.ScopeKey == "" {
		  g.ScopeKey = ScopeKey(g.Chain, g.Asset)
	  }
	   if g.UpdatedAt.IsZero() {
		  g.UpdatedAt = g.CreatedAt
	   }
	  return s.PutGeneration(ctx, g)
   }

   func (s *SQLStore) ListGenerations(ctx context.Context, limit int) ([]*EgressGeneration, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	   if limit <= 0 {
		   limit = 50
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT scope_key, chain, asset, generation, flags_fp, breach_level, tank_digest, decision, reason, updated_at
FROM elg_egress_generation_scopes ORDER BY updated_at DESC LIMIT ?`, limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]*EgressGeneration, 0)
	  for rows.Next() {
		   var g EgressGeneration
		   if err := rows.Scan(&g.ScopeKey, &g.Chain, &g.Asset, &g.Generation, &g.FlagsFP, &g.Breach, &g.TankDigest, &g.Decision, &g.Reason, &g.UpdatedAt); err != nil {
			   return nil, err
		  }
		   out = append(out, &g)
	  }
	   return out, rows.Err()
   }
