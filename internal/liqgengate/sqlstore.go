package liqgengate

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SQLStore 基于 *sql.DB 的实现。未注入 DB 时各方法返回 ErrUnavailable。

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 绑定连接池。
   func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

   func (s *SQLStore) ready() error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  return nil
   }

const (
	sqlInsertGen = `INSERT INTO margin_liq_gens
		(id, borrow_id, user_id, collateral_id, gen, ltv_bps, cap_bps, borrow_amt, collateral_amt, asset, phase, lease_id, hold_id, idem_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetGen = `SELECT id, borrow_id, user_id, collateral_id, gen, ltv_bps, cap_bps, borrow_amt, collateral_amt, asset, phase, lease_id, hold_id, idem_key, created_at, updated_at 
		FROM margin_liq_gens WHERE borrow_id = ?`
	sqlUpdateGen = `UPDATE margin_liq_gens SET gen=?, phase=?, lease_id=?, hold_id=?, ltv_bps=?, cap_bps=?, borrow_amt=?, collateral_amt=?, updated_at=?
		WHERE borrow_id=?`
	sqlListGens = `SELECT id, borrow_id, user_id, collateral_id, gen, ltv_bps, cap_bps, borrow_amt, collateral_amt, asset, phase, lease_id, hold_id, idem_key, created_at, updated_at
		FROM margin_liq_gens ORDER BY updated_at DESC LIMIT ?`
	sqlListGensPhase = `SELECT id, borrow_id, user_id, collateral_id, gen, ltv_bps, cap_bps, borrow_amt, collateral_amt, asset, phase, lease_id, hold_id, idem_key, created_at, updated_at
		FROM margin_liq_gens WHERE phase = ? ORDER BY borrow_id LIMIT ?`
	sqlInsertLease = `INSERT INTO margin_liq_leases
		(id, borrow_id, user_id, collateral_id, hold_id, liq_gen, ltv_bps, cap_bps, asset, amount, state, idem_key, issued_at, expires_at, frozen_at, released_at, consumed_at, revoked_at, revoke_reason, half_proof_id, race_proof_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetLease = `SELECT id, borrow_id, user_id, collateral_id, hold_id, liq_gen, ltv_bps, cap_bps, asset, amount, state, idem_key, issued_at, expires_at, frozen_at, released_at, consumed_at, revoked_at, revoke_reason, half_proof_id, race_proof_id, created_at, updated_at 
		FROM margin_liq_leases WHERE id = ?`
	sqlGetLeaseIdem = `SELECT id, borrow_id, user_id, collateral_id, hold_id, liq_gen, ltv_bps, cap_bps, asset, amount, state, idem_key, issued_at, expires_at, frozen_at, released_at, consumed_at, revoked_at, revoke_reason, half_proof_id, race_proof_id, created_at, updated_at
		FROM margin_liq_leases WHERE idem_key = ?`
	sqlGetActiveLease = `SELECT id, borrow_id, user_id, collateral_id, hold_id, liq_gen, ltv_bps, cap_bps, asset, amount, state, idem_key, issued_at, expires_at, frozen_at, released_at, consumed_at, revoked_at, revoke_reason, half_proof_id, race_proof_id, created_at, updated_at
		FROM margin_liq_leases WHERE borrow_id = ? AND state IN ('issued','freezing','frozen','releasing','half_open') ORDER BY issued_at DESC LIMIT 1`
	sqlUpdateLease = `UPDATE margin_liq_leases SET state=?, hold_id=?, frozen_at=?, released_at=?, consumed_at=?, revoked_at=?, revoke_reason=?, half_proof_id=?, race_proof_id=?, updated_at=? 
		WHERE id=?`
	sqlListExpiredLeases = `SELECT id, borrow_id, user_id, collateral_id, hold_id, liq_gen, ltv_bps, cap_bps, asset, amount, state, idem_key, issued_at, expires_at, frozen_at, released_at, consumed_at, revoked_at, revoke_reason, half_proof_id, race_proof_id, created_at, updated_at
		FROM margin_liq_leases WHERE expires_at <= ? AND state NOT IN ('consumed','expired','revoked') LIMIT ?`
	sqlInsertProof = `INSERT INTO margin_liq_proofs
		(id, kind, borrow_id, user_id, lease_id, liq_gen, peer_gen, reason, state, idem_key, digest, hook_error, phase_at_issue, actor, proven_at, closed_at, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetProof = `SELECT id, kind, borrow_id, user_id, lease_id, liq_gen, peer_gen, reason, state, idem_key, digest, hook_error, phase_at_issue, actor, proven_at, closed_at, note, created_at, updated_at
		FROM margin_liq_proofs WHERE id = ?`
	sqlGetProofIdem = `SELECT id, kind, borrow_id, user_id, lease_id, liq_gen, peer_gen, reason, state, idem_key, digest, hook_error, phase_at_issue, actor, proven_at, closed_at, note, created_at, updated_at
		FROM margin_liq_proofs WHERE idem_key = ?`
	sqlUpdateProof = `UPDATE margin_liq_proofs SET state=?, proven_at=?, closed_at=?, note=?, updated_at=? WHERE id=?`
	sqlInsertAudit = `INSERT INTO margin_liq_audit
		(id, borrow_id, lease_id, from_phase, to_phase, from_lease, to_lease, gen, actor, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
)

  func scanGen(sc interface{ Scan(...any) error }) (*LiqGeneration, error) {
	   var g LiqGeneration
	   err := sc.Scan(&g.ID, &g.BorrowID, &g.UserID, &g.CollateralID, &g.Gen, &g.LTVBPS, &g.CapBPS,
		   &g.BorrowAmt, &g.CollateralAmt, &g.Asset, &g.Phase, &g.LeaseID, &g.HoldID, &g.IdemKey, &g.CreatedAt, &g.UpdatedAt)
	   if err != nil {
		   return nil, err
	   }
	   return &g, nil
  }

  func scanLease(sc interface{ Scan(...any) error }) (*LiqFreezeLease, error) {
	  var l LiqFreezeLease
	  var frozen, released, consumed, revoked sql.NullTime
	  err := sc.Scan(&l.ID, &l.BorrowID, &l.UserID, &l.CollateralID, &l.HoldID, &l.LiqGen, &l.LTVBPS, &l.CapBPS,
		   &l.Asset, &l.Amount, &l.State, &l.IdemKey, &l.IssuedAt, &l.ExpiresAt, &frozen, &released, &consumed, &revoked,
		   &l.RevokeReason, &l.HalfProofID, &l.RaceProofID, &l.CreatedAt, &l.UpdatedAt)
	   if err != nil {
		  return nil, err
	  }
	  if frozen.Valid {
		   l.FrozenAt = &frozen.Time
	  }
	  if released.Valid {
		  l.ReleasedAt = &released.Time
	  }
	   if consumed.Valid {
		   l.ConsumedAt = &consumed.Time
	   }
	  if revoked.Valid {
		   l.RevokedAt = &revoked.Time
	   }
	  return &l, nil
   }

  func scanProof(sc interface{ Scan(...any) error }) (*LiqProof, error) {
	   var p LiqProof
	  var proven, closed sql.NullTime
	   err := sc.Scan(&p.ID, &p.Kind, &p.BorrowID, &p.UserID, &p.LeaseID, &p.LiqGen, &p.PeerGen, &p.Reason,
		   &p.State, &p.IdemKey, &p.Digest, &p.HookError, &p.PhaseAtIssue, &p.Actor, &proven, &closed, &p.Note, &p.CreatedAt, &p.UpdatedAt)
	  if err != nil {
		  return nil, err
	  }
	  if proven.Valid {
		   p.ProvenAt = &proven.Time
	  }
	   if closed.Valid {
		  p.ClosedAt = &closed.Time
	   }
	   return &p, nil
   }

   func nullTime(t *time.Time) any {
	   if t == nil {
		   return nil
	   }
	   return *t
  }

   func (s *SQLStore) InsertGeneration(ctx context.Context, g *LiqGeneration) error {
	   if err := s.ready(); err != nil {
		  return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlInsertGen, g.ID, g.BorrowID, g.UserID, g.CollateralID, g.Gen, g.LTVBPS, g.CapBPS,
		  g.BorrowAmt, g.CollateralAmt, g.Asset, string(g.Phase), g.LeaseID, g.HoldID, g.IdemKey, g.CreatedAt, g.UpdatedAt)
	   return err
  }

   func (s *SQLStore) GetGeneration(ctx context.Context, borrowID string) (*LiqGeneration, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	   }
	  row := s.db.QueryRowContext(ctx, sqlGetGen, borrowID)
	  g, err := scanGen(row)
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	  }
	  return g, err
   }

  func (s *SQLStore) UpdateGeneration(ctx context.Context, g *LiqGeneration) error {
	  if err := s.ready(); err != nil {
		   return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlUpdateGen, g.Gen, string(g.Phase), g.LeaseID, g.HoldID, g.LTVBPS, g.CapBPS,
		  g.BorrowAmt, g.CollateralAmt, g.UpdatedAt, g.BorrowID)
	  return err
   }

  func (s *SQLStore) ListGenerations(ctx context.Context, limit int) ([]*LiqGeneration, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		  limit = 100
	  }
	  rows,
		   err := s.db.QueryContext(ctx, sqlListGens, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []*LiqGeneration
	   for rows.Next() {
		   g, err := scanGen(rows)
		  if err != nil {
			   return nil, err
		  }
		   out = append(out, g)
	  }
	  return out, rows.Err()
   }

   func (s *SQLStore) ListByPhase(ctx context.Context, phase LiqPhase, limit int) ([]*LiqGeneration, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	   if limit <= 0 {
		  limit = 100
	   }
	  rows,
		  err := s.db.QueryContext(ctx, sqlListGensPhase, string(phase), limit)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []*LiqGeneration
	   for rows.Next() {
		   g, err := scanGen(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, g)
	  }
	  return out, rows.Err()
  }

   func (s *SQLStore) InsertLease(ctx context.Context, l *LiqFreezeLease) error {
	   if err := s.ready(); err != nil {
		  return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlInsertLease, l.ID, l.BorrowID, l.UserID, l.CollateralID, l.HoldID, l.LiqGen,
		  l.LTVBPS, l.CapBPS, l.Asset, l.Amount, string(l.State), l.IdemKey, l.IssuedAt, l.ExpiresAt,
		  nullTime(l.FrozenAt), nullTime(l.ReleasedAt), nullTime(l.ConsumedAt), nullTime(l.RevokedAt),
		  l.RevokeReason,
		  l.HalfProofID, l.RaceProofID, l.CreatedAt, l.UpdatedAt)
	   return err
  }

  func (s *SQLStore) GetLeaseByID(ctx context.Context, id string) (*LiqFreezeLease, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	  l, err := scanLease(s.db.QueryRowContext(ctx, sqlGetLease, id))
	   if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	  }
	  return l, err
  }

   func (s *SQLStore) GetLeaseByIdem(ctx context.Context, idemKey string) (*LiqFreezeLease, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	  l, err := scanLease(s.db.QueryRowContext(ctx, sqlGetLeaseIdem, idemKey))
	   if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	   }
	  return l, err
   }

  func (s *SQLStore) GetActiveLease(ctx context.Context, borrowID string) (*LiqFreezeLease, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	   l, err := scanLease(s.db.QueryRowContext(ctx, sqlGetActiveLease, borrowID))
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	   }
	   return l, err
   }

  func (s *SQLStore) UpdateLease(ctx context.Context, l *LiqFreezeLease) error {
	  if err := s.ready(); err != nil {
		  return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlUpdateLease, string(l.State), l.HoldID, nullTime(l.FrozenAt),
		  nullTime(l.ReleasedAt), nullTime(l.ConsumedAt), nullTime(l.RevokedAt), l.RevokeReason,
		  l.HalfProofID, l.RaceProofID, l.UpdatedAt, l.ID)
	   return err
  }

   func (s *SQLStore) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*LiqFreezeLease, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	   if limit <= 0 {
		   limit = 100
	   }
	  rows,
		  err := s.db.QueryContext(ctx, sqlListExpiredLeases, before, limit)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []*LiqFreezeLease
	  for rows.Next() {
		   l, err := scanLease(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, l)
	   }
	  return out, rows.Err()
   }

   func (s *SQLStore) ListActiveLeases(ctx context.Context, limit int) ([]*LiqFreezeLease, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	  }
	  if limit <= 0 {
		   limit = 100
	   }
	   q := `SELECT id, borrow_id, user_id, collateral_id, hold_id, liq_gen, ltv_bps, cap_bps, asset, amount, state, idem_key, issued_at, expires_at, frozen_at, released_at, consumed_at, revoked_at, revoke_reason, half_proof_id, race_proof_id, created_at, updated_at
		FROM margin_liq_leases WHERE state IN ('issued','freezing','frozen','releasing','half_open') ORDER BY id LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, limit)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []*LiqFreezeLease
	  for rows.Next() {
		   l, err := scanLease(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, l)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) ListLeasesByBorrow(ctx context.Context, borrowID string, limit int) ([]*LiqFreezeLease, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		   limit = 50
	  }
	   q := `SELECT id, borrow_id, user_id, collateral_id, hold_id, liq_gen, ltv_bps, cap_bps, asset, amount, state, idem_key, issued_at, expires_at, frozen_at, released_at, consumed_at, revoked_at, revoke_reason, half_proof_id, race_proof_id, created_at, updated_at
		FROM margin_liq_leases WHERE borrow_id = ? ORDER BY issued_at DESC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, borrowID, limit)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []*LiqFreezeLease
	   for rows.Next() {
		  l, err := scanLease(rows)
		  if err != nil {
			   return nil, err
		  }
		  out = append(out, l)
	   }
	  return out, rows.Err()
   }

   func (s *SQLStore) InsertProof(ctx context.Context, p *LiqProof) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertProof, p.ID, string(p.Kind), p.BorrowID, p.UserID, p.LeaseID, p.LiqGen,
		   p.PeerGen, p.Reason, string(p.State), p.IdemKey, p.Digest, p.HookError, p.PhaseAtIssue, p.Actor,
		   nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.Note, p.CreatedAt, p.UpdatedAt)
	   return err
   }

  func (s *SQLStore) GetProofByID(ctx context.Context, id string) (*LiqProof, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	  p, err := scanProof(s.db.QueryRowContext(ctx, sqlGetProof, id))
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	  }
	   return p, err
  }

   func (s *SQLStore) GetProofByIdem(ctx context.Context, idemKey string) (*LiqProof, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	  p, err := scanProof(s.db.QueryRowContext(ctx, sqlGetProofIdem, idemKey))
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	   }
	   return p, err
   }

   func (s *SQLStore) UpdateProof(ctx context.Context, p *LiqProof) error {
	   if err := s.ready(); err != nil {
		  return err
	  }
	   _, err := s.db.ExecContext(ctx, sqlUpdateProof, string(p.State), nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.Note, p.UpdatedAt, p.ID)
	  return err
  }

  func (s *SQLStore) ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*LiqProof, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	  }
	   if limit <= 0 {
		   limit = 100
	  }
	   q := `SELECT id, kind, borrow_id, user_id, lease_id, liq_gen, peer_gen, reason, state, idem_key, digest, hook_error, phase_at_issue, actor, proven_at, closed_at, note, created_at, updated_at
		FROM margin_liq_proofs WHERE state = ? ORDER BY id LIMIT ?`
	  return s.queryProofs(ctx, q, string(state), limit)
  }

  func (s *SQLStore) ListProofsByBorrow(ctx context.Context, borrowID string, limit int) ([]*LiqProof, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	  if limit <= 0 {
		  limit = 50
	   }
	  q := `SELECT id, kind, borrow_id, user_id, lease_id, liq_gen, peer_gen, reason, state, idem_key, digest, hook_error, phase_at_issue, actor, proven_at, closed_at, note, created_at, updated_at
		FROM margin_liq_proofs WHERE borrow_id = ? ORDER BY created_at DESC LIMIT ?`
	  return s.queryProofs(ctx, q, borrowID, limit)
  }

  func (s *SQLStore) ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]*LiqProof, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	  }
	   if limit <= 0 {
		   limit = 100
	   }
	  q := `SELECT id, kind, borrow_id, user_id, lease_id, liq_gen, peer_gen, reason, state, idem_key, digest, hook_error, phase_at_issue, actor, proven_at, closed_at, note, created_at, updated_at
		FROM margin_liq_proofs WHERE kind = ? ORDER BY id LIMIT ?`
	  return s.queryProofs(ctx, q, string(kind), limit)
  }

   func (s *SQLStore) queryProofs(ctx context.Context, q string, a, b any) ([]*LiqProof, error) {
	  rows, err := s.db.QueryContext(ctx, q, a, b)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   var out []*LiqProof
	  for rows.Next() {
		  p, err := scanProof(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, p)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) InsertAudit(ctx context.Context, a *LiqAudit) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertAudit, a.ID, a.BorrowID, a.LeaseID, a.FromPhase, a.ToPhase,
		  a.FromLease, a.ToLease, a.Gen, a.Actor, a.Note, a.CreatedAt)
	   return err
   }

  func (s *SQLStore) ListAuditByBorrow(ctx context.Context, borrowID string, limit int) ([]*LiqAudit, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	  }
	  if limit <= 0 {
		   limit = 50
	  }
	   q := `SELECT id, borrow_id, lease_id, from_phase, to_phase, from_lease, to_lease, gen, actor, note, created_at
		FROM margin_liq_audit WHERE borrow_id = ? ORDER BY created_at DESC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, borrowID, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []*LiqAudit
	   for rows.Next() {
		  var a LiqAudit
		   if err := rows.Scan(&a.ID, &a.BorrowID, &a.LeaseID, &a.FromPhase, &a.ToPhase, &a.FromLease, &a.ToLease, &a.Gen, &a.Actor, &a.Note, &a.CreatedAt); err != nil {
			   return nil, err
		  }
		   out = append(out, &a)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) TryAcquireMutex(_ context.Context, rec *MutexRecord) error {
	  _ = rec
	   return ErrUnavailable
  }

   func (s *SQLStore) ReleaseMutex(_ context.Context, borrowID, owner string) error {
	  _, _ = borrowID, owner
	  return ErrUnavailable
  }

  func (s *SQLStore) GetMutex(_ context.Context, borrowID string) (*MutexRecord, error) {
	  _ = borrowID
	  return nil, ErrUnavailable
   }
