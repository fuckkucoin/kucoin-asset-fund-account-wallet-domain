package destentrygate

import (
	"context"
	"database/sql"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore 基于 persist.SQLDB。
type SQLStore struct {
	db *persist.SQLDB
}

   func NewSQLStore(db *persist.SQLDB) *SQLStore {
	  if db == nil {
		   panic("destentrygate: SQLStore requires non-nil SQLDB")
	   }
	   return &SQLStore{db: db}
   }

  func (s *SQLStore) q(ctx context.Context) persist.Queryer {
	  return s.db.Conn(ctx)
   }

const leaseCols = `lease_id, entry_id, entry_gen, user_id, chain, address, mode, whitelist_only, 
		cooldown_full, state, ttl_seconds, issued_at, bound_at, consumed_at, expired_at, revoked_at,
		revoke_reason, withdraw_ref, idempotency_key, bind_idempotency_key, created_at, updated_at`

   func (s *SQLStore) SaveLease(ctx context.Context, lease DestEntryLease) error {
	  const q = `INSERT INTO dest_entry_leases (` + leaseCols + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.q(ctx).ExecContext(ctx, q,
		  lease.LeaseID, lease.EntryID, lease.EntryGen, lease.UserID, lease.Chain, lease.Address,
		   string(lease.Mode), boolToInt(lease.WhitelistOnly), boolToInt(lease.CooldownFull),
		   string(lease.State), lease.TTLSeconds, lease.IssuedAt, nullTime(lease.BoundAt),
		  nullTime(lease.ConsumedAt), nullTime(lease.ExpiredAt), nullTime(lease.RevokedAt),
		   lease.RevokeReason, lease.WithdrawRef, lease.IdempotencyKey, lease.BindIdemKey,
		  lease.CreatedAt, lease.UpdatedAt,
	   )
	   if err != nil {
		  return WrapStore("save_lease", err)
	   }
	   return nil
   }

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (DestEntryLease, error) {
	  row := s.q(ctx).QueryRowContext(ctx, `SELECT `+leaseCols+` FROM dest_entry_leases WHERE lease_id=?`, leaseID)
	   l, err := scanLease(row)
	  if err == sql.ErrNoRows {
		   return DestEntryLease{}, ErrLeaseNotFound
	   }
	  if err != nil {
		  return DestEntryLease{}, WrapStore("get_lease", err)
	  }
	  return l, nil
   }

   func (s *SQLStore) GetLeaseByIdempotencyKey(ctx context.Context, key string) (DestEntryLease, bool, error) {
	  row := s.q(ctx).QueryRowContext(ctx, `SELECT `+leaseCols+` FROM dest_entry_leases WHERE idempotency_key=? LIMIT 1`, key)
	  l, err := scanLease(row)
	   if err == sql.ErrNoRows {
		   return DestEntryLease{}, false, nil
	  }
	   if err != nil {
		   return DestEntryLease{}, false, WrapStore("get_lease_idem", err)
	   }
	  return l, true, nil
  }

   func (s *SQLStore) UpdateLease(ctx context.Context, lease DestEntryLease) error {
	  const q = `UPDATE dest_entry_leases SET entry_id=?, entry_gen=?, user_id=?, chain=?, address=?,
		mode=?, whitelist_only=?, cooldown_full=?, state=?, ttl_seconds=?, issued_at=?, bound_at=?,
		consumed_at=?, expired_at=?, revoked_at=?, revoke_reason=?, withdraw_ref=?, idempotency_key=?,
		bind_idempotency_key=?, updated_at=? WHERE lease_id=?`
	   res, err := s.q(ctx).ExecContext(ctx, q,
		   lease.EntryID, lease.EntryGen, lease.UserID, lease.Chain, lease.Address, string(lease.Mode),
		  boolToInt(lease.WhitelistOnly), boolToInt(lease.CooldownFull), string(lease.State),
		   lease.TTLSeconds, lease.IssuedAt, nullTime(lease.BoundAt), nullTime(lease.ConsumedAt),
		  nullTime(lease.ExpiredAt), nullTime(lease.RevokedAt), lease.RevokeReason, lease.WithdrawRef,
		  lease.IdempotencyKey, lease.BindIdemKey, lease.UpdatedAt, lease.LeaseID,
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

  func (s *SQLStore) ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]DestEntryLease, error) {
	   if limit <= 0 {
		  limit = 100
	  }
	   rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+leaseCols+` FROM dest_entry_leases WHERE state=? LIMIT ?`, string(state), limit)
	  if err != nil {
		  return nil, WrapStore("list_state", err)
	   }
	  defer rows.Close()
	  return scanLeaseRows(rows)
  }

  func (s *SQLStore) ListExpiredIssued(ctx context.Context, now time.Time, limit int) ([]DestEntryLease, error) {
	   if limit <= 0 {
		  limit = 200
	  }
	   rows, err := s.q(ctx).QueryContext(ctx,
		   `SELECT `+leaseCols+` FROM dest_entry_leases WHERE state IN ('issued','bound') AND DATE_ADD(issued_at, INTERVAL ttl_seconds SECOND) < ? LIMIT ?`,
		  now, limit)
	   if err != nil {
		  return nil, WrapStore("list_expired", err)
	   }
	   defer rows.Close()
	  return scanLeaseRows(rows)
  }

   func (s *SQLStore) ListUnconsumed(ctx context.Context, limit int) ([]DestEntryLease, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   rows, err := s.q(ctx).QueryContext(ctx,
		  `SELECT `+leaseCols+` FROM dest_entry_leases WHERE state IN ('issued','bound') LIMIT ?`, limit)
	  if err != nil {
		   return nil, WrapStore("list_unconsumed", err)
	   }
	   defer rows.Close()
	  return scanLeaseRows(rows)
  }

   func (s *SQLStore) ListByEntry(ctx context.Context, entryID string, limit int) ([]DestEntryLease, error) {
	   if limit <= 0 {
		   limit = 200
	   }
	   rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+leaseCols+` FROM dest_entry_leases WHERE entry_id=? LIMIT ?`, entryID, limit)
	  if err != nil {
		  return nil, WrapStore("list_entry", err)
	  }
	   defer rows.Close()
	   return scanLeaseRows(rows)
   }

  func (s *SQLStore) ListRevocableByEntryGen(ctx context.Context, entryID string, maxGen int64, limit int) ([]DestEntryLease, error) {
	  if limit <= 0 {
		  limit = 200
	   }
	  rows,
		  err := s.q(ctx).QueryContext(ctx,
		   `SELECT `+leaseCols+` FROM dest_entry_leases WHERE entry_id=? AND state IN ('issued','bound') AND entry_gen < ? LIMIT ?`,
		   entryID, maxGen, limit)
	  if err != nil {
		  return nil, WrapStore("list_revocable", err)
	  }
	  defer rows.Close()
	  return scanLeaseRows(rows)
   }

   func (s *SQLStore) SaveGen(ctx context.Context, gen EntryGeneration) error {
	  const q = `INSERT INTO dest_entry_generations (entry_id, user_id, chain, address, entry_gen, mode, bump_reason, bumped_at, bumped_by)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), chain=VALUES(chain), address=VALUES(address),
		entry_gen=VALUES(entry_gen), mode=VALUES(mode), bump_reason=VALUES(bump_reason),
		bumped_at=VALUES(bumped_at), bumped_by=VALUES(bumped_by)`
	   _, err := s.q(ctx).ExecContext(ctx, q, gen.EntryID, gen.UserID, gen.Chain, gen.Address, gen.Gen,
		  string(gen.Mode), gen.BumpReason, gen.BumpedAt, gen.BumpedBy)
	  if err != nil {
		   return WrapStore("save_gen", err)
	  }
	  return nil
   }

   func (s *SQLStore) GetGen(ctx context.Context, entryID string) (EntryGeneration, error) {
	   const q = `SELECT entry_id, user_id, chain, address, entry_gen, mode, bump_reason, bumped_at, bumped_by
		FROM dest_entry_generations WHERE entry_id=?`
	   var g EntryGeneration
	  var mode string
	  err := s.q(ctx).QueryRowContext(ctx,
		   q, entryID).Scan(
		  &g.EntryID, &g.UserID, &g.Chain, &g.Address, &g.Gen, &mode, &g.BumpReason, &g.BumpedAt, &g.BumpedBy,
	   )
	  if err == sql.ErrNoRows {
		  return EntryGeneration{}, ErrGenNotFound
	   }
	  if err != nil {
		   return EntryGeneration{}, WrapStore("get_gen", err)
	   }
	   g.Mode = EntryMode(mode)
	  return g, nil
   }

  func (s *SQLStore) SaveProof(ctx context.Context, proof DestEntryDriftProof) error {
	  const q = `INSERT INTO dest_entry_drift_proofs (
		proof_id, kind, lease_id, entry_id, user_id, chain, address, bind_mode, live_mode,
		bind_gen, live_gen, evidence, detected_at, exported_at, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.q(ctx).ExecContext(ctx, q,
		   proof.ProofID, string(proof.Kind), proof.LeaseID, proof.EntryID, proof.UserID, proof.Chain, proof.Address,
		   string(proof.BindMode), string(proof.LiveMode), proof.BindGen, proof.LiveGen, proof.Evidence,
		  proof.DetectedAt, nullTime(proof.ExportedAt), proof.CreatedAt,
	  )
	   if err != nil {
		  return WrapStore("save_proof", err)
	   }
	  return nil
  }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (DestEntryDriftProof, error) {
	  const q = `SELECT proof_id, kind, lease_id, entry_id, user_id, chain, address, bind_mode, live_mode,
		bind_gen, live_gen, evidence, detected_at, exported_at, created_at
		FROM dest_entry_drift_proofs WHERE proof_id=?`
	  row := s.q(ctx).QueryRowContext(ctx, q, proofID)
	  p, err := scanProof(row)
	  if err == sql.ErrNoRows {
		  return DestEntryDriftProof{},
			  ErrProofNotFound
	   }
	  if err != nil {
		  return DestEntryDriftProof{}, WrapStore("get_proof", err)
	  }
	  return p, nil
   }

   func (s *SQLStore) ListProofs(ctx context.Context, filter ExportProofFilter) ([]DestEntryDriftProof, error) {
	   limit := filter.Limit
	   if limit <= 0 {
		  limit = 100
	   }
	  q := `SELECT proof_id, kind, lease_id, entry_id, user_id, chain, address, bind_mode, live_mode,
		bind_gen, live_gen, evidence, detected_at, exported_at, created_at FROM dest_entry_drift_proofs WHERE 1=1`
	   args := make([]any, 0, 6)
	  if filter.Kind != "" {
		  q += ` AND kind=?`
		   args = append(args,
			  string(filter.Kind))
	   }
	  if filter.EntryID != "" {
		  q += ` AND entry_id=?`
		   args = append(args, filter.EntryID)
	  }
	   if filter.UserID != "" {
		   q += ` AND user_id=?`
		  args = append(args, filter.UserID)
	   }
	  if filter.UnexportedOnly {
		  q += ` AND exported_at IS NULL`
	   }
	   q += ` ORDER BY detected_at DESC LIMIT ?`
	  args = append(args, limit)
	   rows, err := s.q(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, WrapStore("list_proofs", err)
	  }
	  defer rows.Close()
	  out := make([]DestEntryDriftProof, 0)
	   for rows.Next() {
		   p, err := scanProof(rows)
		   if err != nil {
			   return nil, WrapStore("scan_proof", err)
		  }
		  out = append(out, p)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) MarkProofsExported(ctx context.Context, ids []string, at time.Time) error {
	  for _, id := range ids {
		  if _, err := s.q(ctx).ExecContext(ctx, `UPDATE dest_entry_drift_proofs SET exported_at=? WHERE proof_id=?`, at, id); err != nil {
			   return WrapStore("mark_exported", err)
		  }
	   }
	  return nil
  }

   func (s *SQLStore) SaveAudit(ctx context.Context, row DestEntryAudit) error {
	   const q = `INSERT INTO dest_entry_audit (audit_id, lease_id, entry_id, action, actor, detail, created_at)
		VALUES (?,?,?,?,?,?,?)`
	   _,
		   err := s.q(ctx).ExecContext(ctx, q, row.AuditID, row.LeaseID, row.EntryID, string(row.Action), row.Actor, row.Detail, row.CreatedAt)
	   if err != nil {
		  return WrapStore("save_audit", err)
	  }
	   return nil
   }

   func (s *SQLStore) ListAuditByLease(ctx context.Context, leaseID string, limit int) ([]DestEntryAudit, error) {
	   if limit <= 0 {
		  limit = 100
	   }
	   var (
		   rows *sql.Rows
		   err  error
	   )
	  if leaseID == "" {
		  rows, err = s.q(ctx).QueryContext(ctx, `SELECT audit_id, lease_id, entry_id, action, actor, detail, created_at
			FROM dest_entry_audit ORDER BY created_at DESC LIMIT ?`, limit)
	  } else {
		  rows, err = s.q(ctx).QueryContext(ctx, `SELECT audit_id, lease_id, entry_id, action, actor, detail, created_at
			FROM dest_entry_audit WHERE lease_id=? ORDER BY created_at DESC LIMIT ?`, leaseID, limit)
	  }
	   if err != nil {
		  return nil, WrapStore("list_audit", err)
	   }
	   defer rows.Close()
	  out := make([]DestEntryAudit, 0)
	  for rows.Next() {
		   var a DestEntryAudit
		   var act string
		  if err := rows.Scan(&a.AuditID, &a.LeaseID, &a.EntryID, &act, &a.Actor, &a.Detail, &a.CreatedAt); err != nil {
			   return nil, WrapStore("scan_audit", err)
		   }
		  a.Action = AuditAction(act)
		  out = append(out, a)
	   }
	  return out, rows.Err()
  }

type rowScanner interface {
	Scan(dest ...any) error
}

  func scanLease(row rowScanner) (DestEntryLease, error) {
	  var l DestEntryLease
	  var mode, state string
	  var only, cool int
	  var bound, cons, exp, rev sql.NullTime
	  err := row.Scan(
		  &l.LeaseID,
		  &l.EntryID, &l.EntryGen, &l.UserID, &l.Chain, &l.Address, &mode, &only, &cool,
		   &state, &l.TTLSeconds, &l.IssuedAt, &bound, &cons, &exp, &rev, &l.RevokeReason, &l.WithdrawRef,
		   &l.IdempotencyKey, &l.BindIdemKey, &l.CreatedAt, &l.UpdatedAt,
	   )
	  if err != nil {
		  return DestEntryLease{}, err
	   }
	  l.Mode = EntryMode(mode)
	   l.State = LeaseState(state)
	  l.WhitelistOnly = only != 0
	   l.CooldownFull = cool != 0
	  l.BoundAt = timePtr(bound)
	   l.ConsumedAt = timePtr(cons)
	  l.ExpiredAt = timePtr(exp)
	   l.RevokedAt = timePtr(rev)
	   return l, nil
  }

   func scanLeaseRows(rows *sql.Rows) ([]DestEntryLease, error) {
	  out := make([]DestEntryLease, 0)
	  for rows.Next() {
		   l, err := scanLease(rows)
		   if err != nil {
			   return nil, WrapStore("scan_lease", err)
		   }
		  out = append(out, l)
	   }
	   return out, rows.Err()
   }

   func scanProof(row rowScanner) (DestEntryDriftProof, error) {
	   var p DestEntryDriftProof
	   var kind, bm, lm string
	   var exp sql.NullTime
	  err := row.Scan(&p.ProofID, &kind, &p.LeaseID, &p.EntryID, &p.UserID, &p.Chain, &p.Address,
		  &bm, &lm, &p.BindGen, &p.LiveGen, &p.Evidence, &p.DetectedAt, &exp, &p.CreatedAt)
	   if err != nil {
		   return DestEntryDriftProof{}, err
	  }
	  p.Kind = ProofKind(kind)
	   p.BindMode = EntryMode(bm)
	  p.LiveMode = EntryMode(lm)
	  p.ExportedAt = timePtr(exp)
	   return p, nil
  }

   func boolToInt(v bool) int {
	  if v {
		  return 1
	  }
	  return 0
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
	  t := n.Time.UTC()
	   return &t
  }
