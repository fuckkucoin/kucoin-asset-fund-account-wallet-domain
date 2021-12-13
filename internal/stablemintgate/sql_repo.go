package stablemintgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func ph(d persist.Dialect, n int) string {
	  if d == persist.Postgres {
		   return "$" + strconv.Itoa(n)
	   }
	  return "?"
  }

  func nullTime(t *time.Time) sql.NullTime {
	  if t == nil {
		   return sql.NullTime{}
	  }
	  return sql.NullTime{Time: t.UTC(), Valid: true}
   }

   func timePtr(nt sql.NullTime) *time.Time {
	   if !nt.Valid {
		  return nil
	  }
	  t := nt.Time.UTC()
	   return &t
  }

   func itemsJSON(items []string) string {
	   if len(items) == 0 {
		  return "[]"
	   }
	  b, err := json.Marshal(items)
	  if err != nil {
		   return "[]"
	  }
	  return string(b)
   }

  func parseItems(raw string) []string {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return nil
	  }
	  var out []string
	   if err := json.Unmarshal([]byte(raw), &out); err != nil {
		   return nil
	  }
	   return out
   }

// SQLLeaseRepo 租约表。
type SQLLeaseRepo struct{ db *persist.SQLDB }

   func NewSQLLeaseRepo(db *persist.SQLDB) *SQLLeaseRepo { return &SQLLeaseRepo{db: db} }

  func leaseCols() string {
	  return `lease_id, kind, asset, user_id, amount, fingerprint, items_json, fp_expiry, mint_gen, state,
  issued_by, issued_at, expire_at, idempotency_key, journal_id, bound_ledger_id,
  consumed_at, revoked_at, revoke_note, revoke_reason`
   }

  func (r *SQLLeaseRepo) Save(ctx context.Context, l *MintLease) error {
	  if l == nil || l.LeaseID == "" {
		  return ErrInvalidInput
	   }
	   c := r.db.Conn(ctx)
	   var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = c.ExecContext(ctx, ` 
INSERT INTO stable_mint_leases
 (lease_id, kind, asset, user_id, amount, fingerprint, items_json, fp_expiry, mint_gen, state,
  issued_by, issued_at, expire_at, idempotency_key, journal_id, bound_ledger_id,
  consumed_at, revoked_at, revoke_note, revoke_reason)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
ON CONFLICT (lease_id) DO UPDATE SET
  state=EXCLUDED.state, journal_id=EXCLUDED.journal_id, bound_ledger_id=EXCLUDED.bound_ledger_id,
  consumed_at=EXCLUDED.consumed_at, revoked_at=EXCLUDED.revoked_at,
  revoke_note=EXCLUDED.revoke_note, revoke_reason=EXCLUDED.revoke_reason,
  idempotency_key=EXCLUDED.idempotency_key
`, l.LeaseID, string(l.Kind), l.Asset, l.UserID, l.Amount, l.Fingerprint.Digest, itemsJSON(l.Fingerprint.Items),
			  l.Fingerprint.Expiry.UTC(), l.MintGen, string(l.State), l.IssuedBy, l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			   l.Idempotency, l.JournalID, l.BoundLedger, nullTime(l.ConsumedAt), nullTime(l.RevokedAt),
			   l.RevokeNote, l.RevokeReason)
	   } else {
		   _, err = c.ExecContext(ctx, `
INSERT INTO stable_mint_leases
 (lease_id, kind, asset, user_id, amount, fingerprint, items_json, fp_expiry, mint_gen, state,
  issued_by, issued_at, expire_at, idempotency_key, journal_id, bound_ledger_id,
  consumed_at, revoked_at, revoke_note, revoke_reason)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), journal_id=VALUES(journal_id), bound_ledger_id=VALUES(bound_ledger_id),
  consumed_at=VALUES(consumed_at), revoked_at=VALUES(revoked_at),
  revoke_note=VALUES(revoke_note), revoke_reason=VALUES(revoke_reason),
  idempotency_key=VALUES(idempotency_key)
`, l.LeaseID, string(l.Kind), l.Asset, l.UserID, l.Amount, l.Fingerprint.Digest, itemsJSON(l.Fingerprint.Items),
			  l.Fingerprint.Expiry.UTC(), l.MintGen, string(l.State), l.IssuedBy, l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			  l.Idempotency, l.JournalID, l.BoundLedger, nullTime(l.ConsumedAt), nullTime(l.RevokedAt),
			   l.RevokeNote, l.RevokeReason)
	   }
	  return err
   }

  func scanLease(s interface{ Scan(dest ...any) error }) (*MintLease, error) {
	  var l MintLease
	  var kind, state, items string
	   var fpExp time.Time
	  var consumed, revoked sql.NullTime
	   err := s.Scan(
		  &l.LeaseID, &kind, &l.Asset, &l.UserID, &l.Amount, &l.Fingerprint.Digest, &items, &fpExp, &l.MintGen, &state,
		   &l.IssuedBy, &l.IssuedAt, &l.ExpireAt, &l.Idempotency, &l.JournalID, &l.BoundLedger,
		  &consumed, &revoked, &l.RevokeNote, &l.RevokeReason,
	  )
	  if err != nil {
		  if err == sql.ErrNoRows {
			   return nil, ErrLeaseNotFound
		   }
		  return nil, err
	   }
	  l.Kind = LeaseKind(kind)
	  l.State = LeaseState(state)
	  l.Fingerprint.Kind = kind
	  l.Fingerprint.Asset = l.Asset
	   l.Fingerprint.Items = parseItems(items)
	  l.Fingerprint.Expiry = fpExp.UTC()
	  l.Fingerprint.Complete = l.Fingerprint.Digest != ""
	   l.IssuedAt = l.IssuedAt.UTC()
	   l.ExpireAt = l.ExpireAt.UTC()
	  l.ConsumedAt = timePtr(consumed)
	  l.RevokedAt = timePtr(revoked)
	   return &l, nil
  }

  func (r *SQLLeaseRepo) Get(ctx context.Context, leaseID string) (*MintLease, error) {
	   row := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT `+leaseCols()+` FROM stable_mint_leases WHERE lease_id=`+ph(r.db.Dialect, 1), leaseID)
	  return scanLease(row)
   }

   func (r *SQLLeaseRepo) GetByIdem(ctx context.Context, idem string) (*MintLease, error) {
	   row := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT `+leaseCols()+` FROM stable_mint_leases WHERE idempotency_key=`+ph(r.db.Dialect, 1)+` LIMIT 1`, idem)
	   return scanLease(row)
   }

   func (r *SQLLeaseRepo) ListByScope(ctx context.Context, scope string, limit int) ([]*MintLease, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	  parts := strings.SplitN(scope, "|", 2)
	   if len(parts) != 2 {
		  return nil, ErrInvalidInput
	   }
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanLeases(rows)
  }

   func (r *SQLLeaseRepo) ListByState(ctx context.Context, state LeaseState, limit int) ([]*MintLease, error) {
	   if limit <= 0 {
		   limit = 100
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT `+leaseCols()+` FROM stable_mint_leases WHERE state=`+ph(r.db.Dialect, 1)+` LIMIT `+ph(r.db.Dialect, 2),
		  string(state), limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanLeases(rows)
  }

  func scanLeases(rows *sql.Rows) ([]*MintLease, error) {
	  var out []*MintLease
	   for rows.Next() {
		  l, err := scanLease(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, l)
	  }
	  return out, rows.Err()
  }

// SQLHalfRepo 半截证明表。
type SQLHalfRepo struct{ db *persist.SQLDB }

  func NewSQLHalfRepo(db *persist.SQLDB) *SQLHalfRepo { return &SQLHalfRepo{db: db} }

   func halfCols() string {
	  return `half_id, lease_id, journal_id, idempotency_key, kind, asset, user_id, amount, state, cause, digest, created_at, resolved_at`
  }

   func (r *SQLHalfRepo) Save(ctx context.Context, p *StableHalfProof) error {
	   if p == nil || p.HalfID == "" {
		  return ErrInvalidInput
	   }
	  c := r.db.Conn(ctx)
	  var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = c.ExecContext(ctx, `
INSERT INTO stable_half_proofs
 (half_id, lease_id, journal_id, idempotency_key, kind, asset, user_id, amount, state, cause, digest, created_at, resolved_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (half_id) DO UPDATE SET state=EXCLUDED.state, resolved_at=EXCLUDED.resolved_at, digest=EXCLUDED.digest
`, p.HalfID, p.LeaseID, p.JournalID, p.IdemKey, string(p.Kind), p.Asset, p.UserID, p.Amount,
			   string(p.State), p.Cause, p.Digest, p.CreatedAt.UTC(), nullTime(p.ResolvedAt))
	   } else {
		   _, err = c.ExecContext(ctx, `
INSERT INTO stable_half_proofs
 (half_id, lease_id, journal_id, idempotency_key, kind, asset, user_id, amount, state, cause, digest, created_at, resolved_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE state=VALUES(state), resolved_at=VALUES(resolved_at), digest=VALUES(digest)
`, p.HalfID, p.LeaseID, p.JournalID, p.IdemKey, string(p.Kind), p.Asset, p.UserID, p.Amount,
			  string(p.State), p.Cause, p.Digest, p.CreatedAt.UTC(), nullTime(p.ResolvedAt))
	  }
	  return err
   }

  func scanHalf(s interface{ Scan(dest ...any) error }) (*StableHalfProof, error) {
	  var p StableHalfProof
	   var kind, state string
	  var resolved sql.NullTime
	   err := s.Scan(&p.HalfID,
		  &p.LeaseID, &p.JournalID, &p.IdemKey, &kind, &p.Asset, &p.UserID, &p.Amount,
		  &state, &p.Cause, &p.Digest, &p.CreatedAt, &resolved)
	   if err != nil {
		  if err == sql.ErrNoRows {
			  return nil, ErrHalfNotFound
		   }
		   return nil, err
	  }
	   p.Kind = LeaseKind(kind)
	   p.State = HalfState(state)
	   p.CreatedAt = p.CreatedAt.UTC()
	   p.ResolvedAt = timePtr(resolved)
	   return &p, nil
  }

   func (r *SQLHalfRepo) Get(ctx context.Context, halfID string) (*StableHalfProof, error) {
	   return scanHalf(r.db.Conn(ctx).QueryRowContext(ctx, `SELECT `+halfCols()+` FROM stable_half_proofs WHERE half_id=`+ph(r.db.Dialect, 1), halfID))
  }

  func (r *SQLHalfRepo) GetByIdem(ctx context.Context, idem string) (*StableHalfProof, error) {
	   return scanHalf(r.db.Conn(ctx).QueryRowContext(ctx, `SELECT `+halfCols()+` FROM stable_half_proofs WHERE idempotency_key=`+ph(r.db.Dialect, 1)+` LIMIT 1`, idem))
  }

  func (r *SQLHalfRepo) GetByJournal(ctx context.Context, journalID string) (*StableHalfProof, error) {
	   return scanHalf(r.db.Conn(ctx).QueryRowContext(ctx, `SELECT `+halfCols()+` FROM stable_half_proofs WHERE journal_id=`+ph(r.db.Dialect, 1)+` LIMIT 1`, journalID))
  }

  func (r *SQLHalfRepo) ListOpen(ctx context.Context, limit int) ([]*StableHalfProof, error) {
	  if limit <= 0 {
		  limit = 100
	   }
	   rows,
		  err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT `+halfCols()+` FROM stable_half_proofs WHERE state='open' LIMIT `+ph(r.db.Dialect, 1), limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanHalves(rows)
   }

   func (r *SQLHalfRepo) ListByLease(ctx context.Context, leaseID string, limit int) ([]*StableHalfProof, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT `+halfCols()+` FROM stable_half_proofs WHERE lease_id=`+ph(r.db.Dialect, 1)+` LIMIT `+ph(r.db.Dialect, 2),
		  leaseID, limit)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanHalves(rows)
   }

   func scanHalves(rows *sql.Rows) ([]*StableHalfProof, error) {
	  var out []*StableHalfProof
	  for rows.Next() {
		  p, err := scanHalf(rows)
		   if err != nil {
			   return nil, err
		  }
		   out = append(out, p)
	   }
	   return out, rows.Err()
   }

// SQLAuditRepo 审计表。
type SQLAuditRepo struct{ db *persist.SQLDB }

   func NewSQLAuditRepo(db *persist.SQLDB) *SQLAuditRepo { return &SQLAuditRepo{db: db} }

  func (r *SQLAuditRepo) Save(ctx context.Context, a *MintAudit) error {
	   if a == nil || a.AuditID == "" {
		   return ErrInvalidInput
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, `
INSERT INTO stable_mint_audit (audit_id, lease_id, scope, actor, action, detail, outcome, at)
VALUES (`+ph(r.db.Dialect, 1)+`,`+ph(r.db.Dialect, 2)+`,`+ph(r.db.Dialect, 3)+`,`+ph(r.db.Dialect, 4)+`,`+ph(r.db.Dialect, 5)+`,`+ph(r.db.Dialect, 6)+`,`+ph(r.db.Dialect, 7)+`,`+ph(r.db.Dialect, 8)+`)`,
		   a.AuditID, a.LeaseID, a.Scope, a.Actor, a.Action, a.Detail, a.Outcome, a.At.UTC())
	  return err
  }

   func (r *SQLAuditRepo) ListByLease(ctx context.Context, leaseID string, limit int) ([]*MintAudit, error) {
	   return r.list(ctx, `lease_id`, leaseID, limit)
   }

   func (r *SQLAuditRepo) ListByScope(ctx context.Context, scope string, limit int) ([]*MintAudit, error) {
	  return r.list(ctx, `scope`, scope, limit)
  }

   func (r *SQLAuditRepo) list(ctx context.Context, col, val string, limit int) ([]*MintAudit, error) {
	  if limit <= 0 {
		  limit = 50
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT audit_id, lease_id, scope, actor, action, detail, outcome, at FROM stable_mint_audit WHERE `+col+`=`+ph(r.db.Dialect, 1)+` ORDER BY at DESC LIMIT `+ph(r.db.Dialect, 2),
		  val, limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []*MintAudit
	   for rows.Next() {
		   var a MintAudit
		   if err := rows.Scan(&a.AuditID, &a.LeaseID, &a.Scope, &a.Actor, &a.Action, &a.Detail, &a.Outcome, &a.At); err != nil {
			   return nil, err
		   }
		   a.At = a.At.UTC()
		  out = append(out, &a)
	  }
	  return out, rows.Err()
  }

// SQLGenRepo 世代表。

type SQLGenRepo struct{ db *persist.SQLDB }

  func NewSQLGenRepo(db *persist.SQLDB) *SQLGenRepo { return &SQLGenRepo{db: db} }

  func (r *SQLGenRepo) Save(ctx context.Context, g *MintGeneration) error {
	  if g == nil || g.Scope == "" {
		  return ErrInvalidInput
	   }
	   c := r.db.Conn(ctx)
	  var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = c.ExecContext(ctx, `
INSERT INTO stable_mint_gens (scope_key, kind, asset, mint_gen, last_clear, updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (scope_key) DO UPDATE SET mint_gen=EXCLUDED.mint_gen, last_clear=EXCLUDED.last_clear, updated_at=EXCLUDED.updated_at
`, g.Scope, g.Kind, g.Asset, g.MintGen, g.LastClear, g.UpdatedAt.UTC())
	  } else {
		   _, err = c.ExecContext(ctx, `
INSERT INTO stable_mint_gens (scope_key, kind, asset, mint_gen, last_clear, updated_at)
VALUES (?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE mint_gen=VALUES(mint_gen), last_clear=VALUES(last_clear), updated_at=VALUES(updated_at)
`, g.Scope, g.Kind, g.Asset, g.MintGen, g.LastClear, g.UpdatedAt.UTC())
	   }
	  return err
   }

   func (r *SQLGenRepo) Get(ctx context.Context, scope string) (*MintGeneration, error) {
	  row := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT scope_key, kind, asset, mint_gen, last_clear, updated_at FROM stable_mint_gens WHERE scope_key=`+ph(r.db.Dialect, 1), scope)
	   var g MintGeneration
	   err := row.Scan(&g.Scope, &g.Kind, &g.Asset, &g.MintGen, &g.LastClear, &g.UpdatedAt)
	   if err == sql.ErrNoRows {
		   return &MintGeneration{Scope: scope}, nil
	  }
	   if err != nil {
		  return nil, err
	   }
	  g.UpdatedAt = g.UpdatedAt.UTC()
	  return &g, nil
   }

// SQLProofRepo 拒铸证明表。

type SQLProofRepo struct{ db *persist.SQLDB }

   func NewSQLProofRepo(db *persist.SQLDB) *SQLProofRepo { return &SQLProofRepo{db: db} }

  func (r *SQLProofRepo) Save(ctx context.Context, p *DenyProof) error {
	  if p == nil || p.ProofID == "" {
		   return ErrInvalidInput
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, `
INSERT INTO stable_mint_denies (proof_id, lease_id, scope_key, kind, asset, outcome, detail, digest, created_at)
VALUES (`+ph(r.db.Dialect, 1)+`,`+ph(r.db.Dialect, 2)+`,`+ph(r.db.Dialect, 3)+`,`+ph(r.db.Dialect, 4)+`,`+ph(r.db.Dialect, 5)+`,`+ph(r.db.Dialect, 6)+`,`+ph(r.db.Dialect, 7)+`,`+ph(r.db.Dialect, 8)+`,`+ph(r.db.Dialect, 9)+`)`,
		   p.ProofID, p.LeaseID, p.Scope, string(p.Kind), p.Asset, string(p.Outcome), p.Detail, p.Digest, p.CreatedAt.UTC())
	  return err
   }

  func (r *SQLProofRepo) Get(ctx context.Context, proofID string) (*DenyProof, error) {
	  row := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT proof_id, lease_id, scope_key, kind, asset, outcome, detail, digest, created_at FROM stable_mint_denies WHERE proof_id=`+ph(r.db.Dialect, 1), proofID)
	   var p DenyProof
	  var kind, outcome string
	   err := row.Scan(&p.ProofID,
		  &p.LeaseID, &p.Scope, &kind, &p.Asset, &outcome, &p.Detail, &p.Digest, &p.CreatedAt)
	  if err == sql.ErrNoRows {
		   return nil, ErrProofNotFound
	   }
	  if err != nil {
		   return nil, err
	   }
	   p.Kind = LeaseKind(kind)
	  p.Outcome = ProofOutcome(outcome)
	   p.CreatedAt = p.CreatedAt.UTC()
	   return &p, nil
  }

   func (r *SQLProofRepo) ListByScope(ctx context.Context, scope string, limit int) ([]*DenyProof, error) {
	   return r.list(ctx, `scope_key`, scope, limit)
   }

  func (r *SQLProofRepo) ListByOutcome(ctx context.Context, outcome ProofOutcome, limit int) ([]*DenyProof, error) {
	  return r.list(ctx, `outcome`, string(outcome), limit)
  }

   func (r *SQLProofRepo) list(ctx context.Context, col, val string, limit int) ([]*DenyProof, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT proof_id, lease_id, scope_key, kind, asset, outcome, detail, digest, created_at FROM stable_mint_denies WHERE `+col+`=`+ph(r.db.Dialect, 1)+` ORDER BY created_at DESC LIMIT `+ph(r.db.Dialect, 2),
		  val, limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []*DenyProof
	  for rows.Next() {
		   var p DenyProof
		  var kind, outcome string
		   if err := rows.Scan(&p.ProofID, &p.LeaseID, &p.Scope, &kind, &p.Asset, &outcome, &p.Detail, &p.Digest, &p.CreatedAt); err != nil {
			   return nil, err
		  }
		  p.Kind = LeaseKind(kind)
		   p.Outcome = ProofOutcome(outcome)
		  p.CreatedAt = p.CreatedAt.UTC()
		   out = append(out, &p)
	  }
	  return out, rows.Err()
  }
