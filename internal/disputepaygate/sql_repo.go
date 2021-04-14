package disputepaygate

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func ph(d persist.Dialect, n int) string {
	  if d == persist.Postgres {
		   return "$" + itoa(n)
	   }
	   return "?"
  }

  func itoa(n int) string {
	  if n == 0 {
		  return "0"
	  }
	  var b []byte
	  for n > 0 {
		   b = append([]byte{byte('0' + n%10)}, b...)
		   n /= 10
	   }
	  return string(b)
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

// SQLPayLeaseRepo 放款租约表。
type SQLPayLeaseRepo struct{ db *persist.SQLDB }

  func NewSQLPayLeaseRepo(db *persist.SQLDB) *SQLPayLeaseRepo { return &SQLPayLeaseRepo{db: db} }

  func (r *SQLPayLeaseRepo) Save(ctx context.Context, l *DisputePayLease) error {
	   if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	   }
	   q := r.db.Conn(ctx)
	  var err error
	  if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO dispute_pay_leases
 (lease_id, case_id, idempotency_key, user_id, amount, asset, evidence_root, decision,
  pay_gen, state, issued_by, issued_at, expire_at, bound_ledger_id, consumed_at, revoked_at, revoke_note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
ON CONFLICT (lease_id) DO UPDATE SET
  state=EXCLUDED.state, bound_ledger_id=EXCLUDED.bound_ledger_id,
  consumed_at=EXCLUDED.consumed_at, revoked_at=EXCLUDED.revoked_at, revoke_note=EXCLUDED.revoke_note
`, l.LeaseID, l.CaseID, l.Idempotency, l.UserID, l.Amount, l.Asset, l.EvidenceRoot, l.Decision,
			   l.PayGen, string(l.State), l.IssuedBy, l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			   l.BoundLedger, nullTime(l.ConsumedAt), nullTime(l.RevokedAt), l.RevokeNote)
	  } else {
		  _, err = q.ExecContext(ctx, `
INSERT INTO dispute_pay_leases
 (lease_id, case_id, idempotency_key, user_id, amount, asset, evidence_root, decision,
  pay_gen, state, issued_by, issued_at, expire_at, bound_ledger_id, consumed_at, revoked_at, revoke_note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), bound_ledger_id=VALUES(bound_ledger_id),
  consumed_at=VALUES(consumed_at), revoked_at=VALUES(revoked_at), revoke_note=VALUES(revoke_note)
`, l.LeaseID, l.CaseID, l.Idempotency, l.UserID, l.Amount, l.Asset, l.EvidenceRoot, l.Decision,
			  l.PayGen,
			  string(l.State), l.IssuedBy, l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			  l.BoundLedger, nullTime(l.ConsumedAt), nullTime(l.RevokedAt), l.RevokeNote)
	   }
	  return err
   }

   func payCols() string {
	   return `lease_id, case_id, idempotency_key, user_id, amount, asset, evidence_root, decision,
  pay_gen, state, issued_by, issued_at, expire_at, bound_ledger_id, consumed_at, revoked_at, revoke_note`
   }

   func scanPay(row *sql.Row) (*DisputePayLease, error) {
	   var l DisputePayLease
	  var state string
	   var cons, rev sql.NullTime
	   err := row.Scan(&l.LeaseID, &l.CaseID, &l.Idempotency, &l.UserID, &l.Amount, &l.Asset,
		   &l.EvidenceRoot, &l.Decision, &l.PayGen, &state, &l.IssuedBy, &l.IssuedAt, &l.ExpireAt,
		  &l.BoundLedger, &cons, &rev, &l.RevokeNote)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrLeaseNotFound
	  }
	   if err != nil {
		   return nil, err
	  }
	  l.State = LeaseState(state)
	   l.IssuedAt = l.IssuedAt.UTC()
	  l.ExpireAt = l.ExpireAt.UTC()
	  l.ConsumedAt = timePtr(cons)
	   l.RevokedAt = timePtr(rev)
	   return &l, nil
   }

  func scanPays(rows *sql.Rows) ([]*DisputePayLease, error) {
	   var out []*DisputePayLease
	  for rows.Next() {
		  var l DisputePayLease
		  var state string
		   var cons, rev sql.NullTime
		   if err := rows.Scan(&l.LeaseID, &l.CaseID, &l.Idempotency, &l.UserID, &l.Amount, &l.Asset,
			   &l.EvidenceRoot, &l.Decision, &l.PayGen, &state, &l.IssuedBy, &l.IssuedAt, &l.ExpireAt,
			  &l.BoundLedger, &cons, &rev, &l.RevokeNote); err != nil {
			   return nil, err
		  }
		  l.State = LeaseState(state)
		   l.IssuedAt = l.IssuedAt.UTC()
		   l.ExpireAt = l.ExpireAt.UTC()
		   l.ConsumedAt = timePtr(cons)
		   l.RevokedAt = timePtr(rev)
		  out = append(out, &l)
	   }
	   return out, rows.Err()
   }

  func (r *SQLPayLeaseRepo) Get(ctx context.Context, leaseID string) (*DisputePayLease, error) {
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `SELECT `+payCols()+` FROM dispute_pay_leases WHERE lease_id=`+ph(r.db.Dialect, 1), leaseID)
	  return scanPay(row)
   }

   func (r *SQLPayLeaseRepo) GetByCase(ctx context.Context, caseID string) (*DisputePayLease, error) {
	  q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `SELECT `+payCols()+` FROM dispute_pay_leases WHERE case_id=`+ph(r.db.Dialect, 1)+` ORDER BY issued_at DESC LIMIT 1`, caseID)
	   return scanPay(row)
  }

  func (r *SQLPayLeaseRepo) GetByIdem(ctx context.Context, idem string) (*DisputePayLease, error) {
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `SELECT `+payCols()+` FROM dispute_pay_leases WHERE idempotency_key=`+ph(r.db.Dialect, 1)+` LIMIT 1`, idem)
	   return scanPay(row)
  }

  func (r *SQLPayLeaseRepo) ListIssuedExpired(ctx context.Context, now time.Time, limit int) ([]*DisputePayLease, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   q := r.db.Conn(ctx)
	  var rows *sql.Rows
	   var err error
	  if r.db.Dialect == persist.Postgres {
		  rows, err = q.QueryContext(ctx, `SELECT `+payCols()+` FROM dispute_pay_leases WHERE state='pay_issued' AND expire_at<=$1 LIMIT $2`, now.UTC(), limit)
	  } else {
		   rows, err = q.QueryContext(ctx, `SELECT `+payCols()+` FROM dispute_pay_leases WHERE state='pay_issued' AND expire_at<=? LIMIT ?`, now.UTC(), limit)
	   }
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanPays(rows)
  }

   func (r *SQLPayLeaseRepo) ListByState(ctx context.Context, state LeaseState, limit int) ([]*DisputePayLease, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	   q := r.db.Conn(ctx)
	  var rows *sql.Rows
	  var err error
	  if r.db.Dialect == persist.Postgres {
		   rows, err = q.QueryContext(ctx, `SELECT `+payCols()+` FROM dispute_pay_leases WHERE state=$1 LIMIT $2`, string(state), limit)
	  } else {
		   rows, err = q.QueryContext(ctx, `SELECT `+payCols()+` FROM dispute_pay_leases WHERE state=? LIMIT ?`, string(state), limit)
	  }
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanPays(rows)
  }

// SQLClawLeaseRepo 扣回租约表。

type SQLClawLeaseRepo struct{ db *persist.SQLDB }

   func NewSQLClawLeaseRepo(db *persist.SQLDB) *SQLClawLeaseRepo { return &SQLClawLeaseRepo{db: db} }

  func clawCols() string {
	   return `lease_id, case_id, idempotency_key, user_id, amount, asset, evidence_root, decision,
  pay_gen, state, release_ref, issued_by, issued_at, expire_at, bound_ledger_id, consumed_at, revoked_at, revoke_note`
   }

   func (r *SQLClawLeaseRepo) Save(ctx context.Context, l *ClawLease) error {
	  if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	  }
	   q := r.db.Conn(ctx)
	   var err error
	  if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, `
INSERT INTO dispute_claw_leases
 (lease_id, case_id, idempotency_key, user_id, amount, asset, evidence_root, decision,
  pay_gen, state, release_ref, issued_by, issued_at, expire_at, bound_ledger_id, consumed_at, revoked_at, revoke_note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
ON CONFLICT (lease_id) DO UPDATE SET
  state=EXCLUDED.state, release_ref=EXCLUDED.release_ref, bound_ledger_id=EXCLUDED.bound_ledger_id,
  consumed_at=EXCLUDED.consumed_at, revoked_at=EXCLUDED.revoked_at, revoke_note=EXCLUDED.revoke_note
`, l.LeaseID, l.CaseID, l.Idempotency, l.UserID, l.Amount, l.Asset, l.EvidenceRoot, l.Decision,
			   l.PayGen, string(l.State), l.ReleaseRef, l.IssuedBy, l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			   l.BoundLedger, nullTime(l.ConsumedAt), nullTime(l.RevokedAt), l.RevokeNote)
	   } else {
		   _, err = q.ExecContext(ctx, `
INSERT INTO dispute_claw_leases
 (lease_id, case_id, idempotency_key, user_id, amount, asset, evidence_root, decision,
  pay_gen, state, release_ref, issued_by, issued_at, expire_at, bound_ledger_id, consumed_at, revoked_at, revoke_note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), release_ref=VALUES(release_ref), bound_ledger_id=VALUES(bound_ledger_id),
  consumed_at=VALUES(consumed_at), revoked_at=VALUES(revoked_at), revoke_note=VALUES(revoke_note)
`, l.LeaseID, l.CaseID, l.Idempotency, l.UserID, l.Amount, l.Asset, l.EvidenceRoot, l.Decision,
			  l.PayGen, string(l.State), l.ReleaseRef, l.IssuedBy, l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			  l.BoundLedger, nullTime(l.ConsumedAt), nullTime(l.RevokedAt), l.RevokeNote)
	   }
	   return err
  }

  func scanClaw(row *sql.Row) (*ClawLease, error) {
	  var l ClawLease
	  var state string
	   var cons, rev sql.NullTime
	  err := row.Scan(&l.LeaseID, &l.CaseID, &l.Idempotency, &l.UserID, &l.Amount, &l.Asset,
		   &l.EvidenceRoot, &l.Decision, &l.PayGen, &state, &l.ReleaseRef, &l.IssuedBy, &l.IssuedAt, &l.ExpireAt,
		  &l.BoundLedger, &cons, &rev, &l.RevokeNote)
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrLeaseNotFound
	  }
	  if err != nil {
		  return nil, err
	  }
	   l.State = LeaseState(state)
	  l.IssuedAt = l.IssuedAt.UTC()
	  l.ExpireAt = l.ExpireAt.UTC()
	  l.ConsumedAt = timePtr(cons)
	  l.RevokedAt = timePtr(rev)
	   return &l, nil
  }

   func scanClaws(rows *sql.Rows) ([]*ClawLease, error) {
	   var out []*ClawLease
	  for rows.Next() {
		   var l ClawLease
		  var state string
		   var cons, rev sql.NullTime
		  if err := rows.Scan(&l.LeaseID, &l.CaseID, &l.Idempotency, &l.UserID, &l.Amount, &l.Asset,
			  &l.EvidenceRoot,
			   &l.Decision, &l.PayGen, &state, &l.ReleaseRef, &l.IssuedBy, &l.IssuedAt, &l.ExpireAt,
			  &l.BoundLedger, &cons, &rev, &l.RevokeNote); err != nil {
			   return nil, err
		   }
		   l.State = LeaseState(state)
		  l.IssuedAt = l.IssuedAt.UTC()
		  l.ExpireAt = l.ExpireAt.UTC()
		   l.ConsumedAt = timePtr(cons)
		  l.RevokedAt = timePtr(rev)
		   out = append(out, &l)
	  }
	   return out, rows.Err()
  }

   func (r *SQLClawLeaseRepo) Get(ctx context.Context, leaseID string) (*ClawLease, error) {
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `SELECT `+clawCols()+` FROM dispute_claw_leases WHERE lease_id=`+ph(r.db.Dialect, 1), leaseID)
	  return scanClaw(row)
   }

  func (r *SQLClawLeaseRepo) GetByCase(ctx context.Context, caseID string) (*ClawLease, error) {
	   q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `SELECT `+clawCols()+` FROM dispute_claw_leases WHERE case_id=`+ph(r.db.Dialect, 1)+` ORDER BY issued_at DESC LIMIT 1`, caseID)
	  return scanClaw(row)
   }

   func (r *SQLClawLeaseRepo) ListIssuedExpired(ctx context.Context, now time.Time, limit int) ([]*ClawLease, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	  q := r.db.Conn(ctx)
	   var rows *sql.Rows
	  var err error
	  if r.db.Dialect == persist.Postgres {
		  rows, err = q.QueryContext(ctx, `SELECT `+clawCols()+` FROM dispute_claw_leases WHERE state='claw_issued' AND expire_at<=$1 LIMIT $2`, now.UTC(), limit)
	  } else {
		  rows, err = q.QueryContext(ctx, `SELECT `+clawCols()+` FROM dispute_claw_leases WHERE state='claw_issued' AND expire_at<=? LIMIT ?`, now.UTC(), limit)
	   }
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanClaws(rows)
   }

   func (r *SQLClawLeaseRepo) ListByState(ctx context.Context, state LeaseState, limit int) ([]*ClawLease, error) {
	   if limit <= 0 {
		   limit = 100
	  }
	  q := r.db.Conn(ctx)
	   var rows *sql.Rows
	   var err error
	  if r.db.Dialect == persist.Postgres {
		  rows, err = q.QueryContext(ctx, `SELECT `+clawCols()+` FROM dispute_claw_leases WHERE state=$1 LIMIT $2`, string(state), limit)
	  } else {
		   rows, err = q.QueryContext(ctx, `SELECT `+clawCols()+` FROM dispute_claw_leases WHERE state=? LIMIT ?`, string(state), limit)
	   }
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanClaws(rows)
  }

// SQLProofRepo 只增证明表。

type SQLProofRepo struct{ db *persist.SQLDB }

  func NewSQLProofRepo(db *persist.SQLDB) *SQLProofRepo { return &SQLProofRepo{db: db} }

   func (r *SQLProofRepo) Save(ctx context.Context, p *DisputePayProof) error {
	   if p == nil || p.ProofID == "" {
		  return ErrInvalidInput
	  }
	  q := r.db.Conn(ctx)
	   var err error
	  if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO dispute_pay_proofs
 (proof_id, case_id, pay_gen, ledger_id, lease_id, kind, outcome, detail, digest, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (proof_id) DO NOTHING
`, p.ProofID, p.CaseID, p.PayGen, p.LedgerID, p.LeaseID, string(p.Kind), string(p.Outcome), p.Detail, p.Digest, p.CreatedAt.UTC())
	   } else {
		   _, err = q.ExecContext(ctx, `
INSERT IGNORE INTO dispute_pay_proofs
 (proof_id, case_id, pay_gen, ledger_id, lease_id, kind, outcome, detail, digest, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
`, p.ProofID, p.CaseID, p.PayGen, p.LedgerID, p.LeaseID, string(p.Kind), string(p.Outcome), p.Detail, p.Digest, p.CreatedAt.UTC())
	  }
	   return err
   }

   func scanProof(row *sql.Row) (*DisputePayProof, error) {
	   var p DisputePayProof
	   var kind, outcome string
	  err := row.Scan(&p.ProofID, &p.CaseID, &p.PayGen, &p.LedgerID, &p.LeaseID, &kind, &outcome, &p.Detail, &p.Digest, &p.CreatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
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

  func scanProofs(rows *sql.Rows) ([]*DisputePayProof, error) {
	  var out []*DisputePayProof
	   for rows.Next() {
		  var p DisputePayProof
		   var kind, outcome string
		  if err := rows.Scan(&p.ProofID, &p.CaseID, &p.PayGen, &p.LedgerID, &p.LeaseID, &kind, &outcome, &p.Detail, &p.Digest, &p.CreatedAt); err != nil {
			  return nil, err
		   }
		  p.Kind = LeaseKind(kind)
		   p.Outcome = ProofOutcome(outcome)
		  p.CreatedAt = p.CreatedAt.UTC()
		   out = append(out, &p)
	  }
	  return out, rows.Err()
   }

   func proofCols() string {
	   return `proof_id, case_id, pay_gen, ledger_id, lease_id, kind, outcome, detail, digest, created_at`
   }

   func (r *SQLProofRepo) Get(ctx context.Context, proofID string) (*DisputePayProof, error) {
	   q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `SELECT `+proofCols()+` FROM dispute_pay_proofs WHERE proof_id=`+ph(r.db.Dialect, 1), proofID)
	   return scanProof(row)
   }

   func (r *SQLProofRepo) ListByCase(ctx context.Context, caseID string, limit int) ([]*DisputePayProof, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	  q := r.db.Conn(ctx)
	   var rows *sql.Rows
	  var err error
	   if r.db.Dialect == persist.Postgres {
		  rows, err = q.QueryContext(ctx, `SELECT `+proofCols()+` FROM dispute_pay_proofs WHERE case_id=$1 ORDER BY created_at DESC LIMIT $2`, caseID, limit)
	  } else {
		  rows, err = q.QueryContext(ctx, `SELECT `+proofCols()+` FROM dispute_pay_proofs WHERE case_id=? ORDER BY created_at DESC LIMIT ?`, caseID, limit)
	   }
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanProofs(rows)
  }

  func (r *SQLProofRepo) ListByOutcome(ctx context.Context,
	  outcome ProofOutcome, limit int) ([]*DisputePayProof, error) {
	  if limit <= 0 {
		   limit = 50
	   }
	   q := r.db.Conn(ctx)
	  var rows *sql.Rows
	   var err error
	  if r.db.Dialect == persist.Postgres {
		  rows, err = q.QueryContext(ctx, `SELECT `+proofCols()+` FROM dispute_pay_proofs WHERE outcome=$1 ORDER BY created_at DESC LIMIT $2`, string(outcome), limit)
	  } else {
		  rows, err = q.QueryContext(ctx, `SELECT `+proofCols()+` FROM dispute_pay_proofs WHERE outcome=? ORDER BY created_at DESC LIMIT ?`, string(outcome), limit)
	   }
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   return scanProofs(rows)
   }

// SQLAuditRepo 审计表。
type SQLAuditRepo struct{ db *persist.SQLDB }

  func NewSQLAuditRepo(db *persist.SQLDB) *SQLAuditRepo { return &SQLAuditRepo{db: db} }

   func (r *SQLAuditRepo) Save(ctx context.Context, a *AuditRow) error {
	   if a == nil || a.AuditID == "" {
		   return ErrInvalidInput
	   }
	   q := r.db.Conn(ctx)
	  var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO dispute_pay_audit (audit_id, case_id, lease_id, actor, action, detail, at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (audit_id) DO NOTHING
`, a.AuditID, a.CaseID, a.LeaseID, a.Actor, a.Action, a.Detail, a.At.UTC())
	  } else {
		  _, err = q.ExecContext(ctx, `
INSERT IGNORE INTO dispute_pay_audit (audit_id, case_id, lease_id, actor, action, detail, at)
VALUES (?,?,?,?,?,?,?)
`, a.AuditID, a.CaseID, a.LeaseID, a.Actor, a.Action, a.Detail, a.At.UTC())
	  }
	   return err
  }

   func (r *SQLAuditRepo) ListByCase(ctx context.Context, caseID string, limit int) ([]*AuditRow, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	  q := r.db.Conn(ctx)
	  var rows *sql.Rows
	   var err error
	   if r.db.Dialect == persist.Postgres {
		  rows, err = q.QueryContext(ctx, `SELECT audit_id, case_id, lease_id, actor, action, detail, at FROM dispute_pay_audit WHERE case_id=$1 ORDER BY at DESC LIMIT $2`, caseID, limit)
	  } else {
		   rows, err = q.QueryContext(ctx, `SELECT audit_id, case_id, lease_id, actor, action, detail, at FROM dispute_pay_audit WHERE case_id=? ORDER BY at DESC LIMIT ?`, caseID, limit)
	   }
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  var out []*AuditRow
	   for rows.Next() {
		   var a AuditRow
		   if err := rows.Scan(&a.AuditID, &a.CaseID, &a.LeaseID, &a.Actor, &a.Action, &a.Detail, &a.At); err != nil {
			   return nil, err
		   }
		   a.At = a.At.UTC()
		   out = append(out, &a)
	  }
	   return out, rows.Err()
  }

// SQLGenRepo 世代镜像。
type SQLGenRepo struct{ db *persist.SQLDB }

  func NewSQLGenRepo(db *persist.SQLDB) *SQLGenRepo { return &SQLGenRepo{db: db} }

  func (r *SQLGenRepo) Save(ctx context.Context, g *CaseGeneration) error {
	   if g == nil || g.CaseID == "" {
		  return ErrInvalidInput
	   }
	  q := r.db.Conn(ctx)
	   var err error
	   if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, `
INSERT INTO dispute_case_generations (case_id, pay_gen, evidence_root, updated_at)
VALUES ($1,$2,$3,$4)
ON CONFLICT (case_id) DO UPDATE SET pay_gen=EXCLUDED.pay_gen, evidence_root=EXCLUDED.evidence_root, updated_at=EXCLUDED.updated_at
`, g.CaseID, g.PayGen, g.EvidenceRoot, g.UpdatedAt.UTC())
	   } else {
		  _, err = q.ExecContext(ctx, `
INSERT INTO dispute_case_generations (case_id, pay_gen, evidence_root, updated_at)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE pay_gen=VALUES(pay_gen), evidence_root=VALUES(evidence_root), updated_at=VALUES(updated_at)
`, g.CaseID, g.PayGen, g.EvidenceRoot, g.UpdatedAt.UTC())
	  }
	   return err
   }

  func (r *SQLGenRepo) Get(ctx context.Context, caseID string) (*CaseGeneration, error) {
	  q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `SELECT case_id, pay_gen, evidence_root, updated_at FROM dispute_case_generations WHERE case_id=`+ph(r.db.Dialect, 1), caseID)
	   var g CaseGeneration
	   err := row.Scan(&g.CaseID, &g.PayGen, &g.EvidenceRoot, &g.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrCaseNotFound
	   }
	   if err != nil {
		  return nil, err
	   }
	  g.UpdatedAt = g.UpdatedAt.UTC()
	  return &g, nil
  }
