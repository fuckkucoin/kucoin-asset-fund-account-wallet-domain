package hmacwritelease

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ──────────────────────────────────────────────────────────────────────────────
// SQLLeaseRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLLeaseRepo 基于 persist.SQLDB 的 HmacWriteLease 存储实现。

type SQLLeaseRepo struct {
	db *persist.SQLDB
}

// NewSQLLeaseRepo 构造 SQL 租约仓储。

  func NewSQLLeaseRepo(db *persist.SQLDB) *SQLLeaseRepo {
	   return &SQLLeaseRepo{db: db}
   }

   func (r *SQLLeaseRepo) SaveLease(ctx context.Context, l *HmacWriteLease) error {
	   if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	  }
	   q := r.db.Conn(ctx)
	   var err error
	  if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, ` 
INSERT INTO hmac_write_leases
  (lease_id, key_id, nonce, ts, method_path, halt_gen, state,
   issued_at, expire_at, bound_at, consumed_at, revoked_at, revoke_note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (lease_id) DO UPDATE SET
  state=EXCLUDED.state,
  bound_at=EXCLUDED.bound_at,
  consumed_at=EXCLUDED.consumed_at,
  revoked_at=EXCLUDED.revoked_at,
  revoke_note=EXCLUDED.revoke_note
`,
			  l.LeaseID, l.KeyID, l.Nonce, l.TS, l.MethodPath, l.HaltGen, string(l.State),
			  l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			  nullableTimePtr(l.BoundAt), nullableTimePtr(l.ConsumedAt), nullableTimePtr(l.RevokedAt),
			  l.RevokeNote,
		   )
	  } else {
		   _, err = q.ExecContext(ctx, `
INSERT INTO hmac_write_leases
  (lease_id, key_id, nonce, ts, method_path, halt_gen, state,
   issued_at, expire_at, bound_at, consumed_at, revoked_at, revoke_note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state),
  bound_at=VALUES(bound_at),
  consumed_at=VALUES(consumed_at),
  revoked_at=VALUES(revoked_at),
  revoke_note=VALUES(revoke_note)
`,
			  l.LeaseID, l.KeyID, l.Nonce, l.TS, l.MethodPath, l.HaltGen, string(l.State),
			  l.IssuedAt.UTC(), l.ExpireAt.UTC(),
			  nullableTimePtr(l.BoundAt), nullableTimePtr(l.ConsumedAt), nullableTimePtr(l.RevokedAt),
			  l.RevokeNote,
		  )
	   }
	  return err
   }

  func (r *SQLLeaseRepo) GetLease(ctx context.Context, leaseID string) (*HmacWriteLease, error) {
	   q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, ` 
SELECT lease_id, key_id, nonce, ts, method_path, halt_gen, state,
       issued_at, expire_at, bound_at, consumed_at, revoked_at, revoke_note
FROM hmac_write_leases WHERE lease_id = `+r.ph(1),
		  leaseID,
	   )
	   return scanLease(row)
   }

   func (r *SQLLeaseRepo) ListByState(ctx context.Context, state LeaseState, limit int) ([]*HmacWriteLease, error) {
	  q := r.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, `
SELECT lease_id, key_id, nonce, ts, method_path, halt_gen, state,
       issued_at, expire_at, bound_at, consumed_at, revoked_at, revoke_note
FROM hmac_write_leases WHERE state = `+r.ph(1)+` LIMIT `+r.ph(2),
		  string(state), limit,
	  )
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanLeases(rows)
   }

  func (r *SQLLeaseRepo) ListIssuedExpired(ctx context.Context, limit int) ([]*HmacWriteLease, error) {
	  q := r.db.Conn(ctx)
	   now := time.Now().UTC()
	  rows, err := q.QueryContext(ctx, ` 
SELECT lease_id, key_id, nonce, ts, method_path, halt_gen, state,
       issued_at, expire_at, bound_at, consumed_at, revoked_at, revoke_note
FROM hmac_write_leases
WHERE state IN ('issued','bound') AND expire_at <= `+r.ph(1)+`
LIMIT `+r.ph(2),
		  now, limit,
	   )
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanLeases(rows)
   }

   func (r *SQLLeaseRepo) ListByHaltGenLT(ctx context.Context, currentGen int64, limit int) ([]*HmacWriteLease, error) {
	  q := r.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, `
SELECT lease_id, key_id, nonce, ts, method_path, halt_gen, state,
       issued_at, expire_at, bound_at, consumed_at, revoked_at, revoke_note
FROM hmac_write_leases
WHERE halt_gen < `+r.ph(1)+` AND state NOT IN ('consumed','expired','revoked')
LIMIT `+r.ph(2),
		  currentGen, limit,
	   )
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanLeases(rows)
   }

  func (r *SQLLeaseRepo) ph(n int) string {
	  if r.db.Dialect == persist.Postgres {
		   return "$" + itoa(n)
	   }
	   return "?"
  }

  func scanLease(row *sql.Row) (*HmacWriteLease, error) {
	   var l HmacWriteLease
	  var stateStr string
	   var boundAt, consumedAt, revokedAt sql.NullTime
	  err := row.Scan(
		  &l.LeaseID, &l.KeyID, &l.Nonce, &l.TS, &l.MethodPath, &l.HaltGen, &stateStr,
		   &l.IssuedAt, &l.ExpireAt,
		  &boundAt, &consumedAt, &revokedAt,
		  &l.RevokeNote,
	  )
	   if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrLeaseNotFound
	  }
	   if err != nil {
		  return nil, err
	  }
	   l.State = LeaseState(stateStr)
	   l.IssuedAt = l.IssuedAt.UTC()
	   l.ExpireAt = l.ExpireAt.UTC()
	  if boundAt.Valid {
		  t := boundAt.Time.UTC()
		   l.BoundAt = &t
	   }
	   if consumedAt.Valid {
		   t := consumedAt.Time.UTC()
		  l.ConsumedAt = &t
	   }
	  if revokedAt.Valid {
		  t := revokedAt.Time.UTC()
		   l.RevokedAt = &t
	  }
	  return &l, nil
  }

  func scanLeases(rows *sql.Rows) ([]*HmacWriteLease, error) {
	  var out []*HmacWriteLease
	   for rows.Next() {
		  var l HmacWriteLease
		   var stateStr string
		  var boundAt, consumedAt, revokedAt sql.NullTime
		  if err := rows.Scan(
			   &l.LeaseID, &l.KeyID, &l.Nonce, &l.TS, &l.MethodPath, &l.HaltGen, &stateStr,
			   &l.IssuedAt, &l.ExpireAt,
			   &boundAt, &consumedAt, &revokedAt,
			  &l.RevokeNote,
		   ); err != nil {
			  return nil, err
		  }
		   l.State = LeaseState(stateStr)
		   l.IssuedAt = l.IssuedAt.UTC()
		   l.ExpireAt = l.ExpireAt.UTC()
		  if boundAt.Valid {
			   t := boundAt.Time.UTC()
			   l.BoundAt = &t
		  }
		   if consumedAt.Valid {
			  t := consumedAt.Time.UTC()
			  l.ConsumedAt = &t
		  }
		  if revokedAt.Valid {
			   t := revokedAt.Time.UTC()
			   l.RevokedAt = &t
		  }
		   out = append(out, &l)
	  }
	   return out, rows.Err()
  }

// ──────────────────────────────────────────────────────────────────────────────
// SQLNonceSeenRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLNonceSeenRepo 基于 persist.SQLDB 的 HmacNonceSeen 存储实现。
type SQLNonceSeenRepo struct {
	db *persist.SQLDB
}

// NewSQLNonceSeenRepo 构造 SQL nonce 防重放仓储。
  func NewSQLNonceSeenRepo(db *persist.SQLDB) *SQLNonceSeenRepo {
	  return &SQLNonceSeenRepo{db: db}
   }

  func (r *SQLNonceSeenRepo) SaveNonceSeen(ctx context.Context, s *HmacNonceSeen) error {
	   if s == nil || s.SeenID == "" {
		  return ErrInvalidInput
	  }
	  q := r.db.Conn(ctx)
	   var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO hmac_nonce_seen (seen_id, key_id, nonce, lease_id, seen_at, expire_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (seen_id) DO NOTHING
`,
			   s.SeenID, s.KeyID, s.Nonce, s.LeaseID, s.SeenAt.UTC(), s.ExpireAt.UTC(),
		   )
	  } else {
		   _, err = q.ExecContext(ctx, `
INSERT IGNORE INTO hmac_nonce_seen (seen_id, key_id, nonce, lease_id, seen_at, expire_at)
VALUES (?,?,?,?,?,?)
`,
			  s.SeenID, s.KeyID, s.Nonce, s.LeaseID, s.SeenAt.UTC(), s.ExpireAt.UTC(),
		   )
	   }
	   return err
   }

   func (r *SQLNonceSeenRepo) GetNonceSeen(ctx context.Context, seenID string) (*HmacNonceSeen, error) {
	   q := r.db.Conn(ctx)
	   ph := "?"
	  if r.db.Dialect == persist.Postgres {
		   ph = "$1"
	   }
	   row := q.QueryRowContext(ctx, `
SELECT seen_id, key_id, nonce, lease_id, seen_at, expire_at
FROM hmac_nonce_seen WHERE seen_id = `+ph,
		   seenID,
	  )
	  var s HmacNonceSeen
	  err := row.Scan(&s.SeenID, &s.KeyID, &s.Nonce, &s.LeaseID, &s.SeenAt, &s.ExpireAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrLeaseNotFound
	  }
	   if err != nil {
		   return nil, err
	   }
	   s.SeenAt = s.SeenAt.UTC()
	  s.ExpireAt = s.ExpireAt.UTC()
	  return &s, nil
  }

  func (r *SQLNonceSeenRepo) DeleteExpiredNonces(ctx context.Context, limit int) (int, error) {
	  q := r.db.Conn(ctx)
	  now := time.Now().UTC()
	   var res sql.Result
	   var err error
	   if r.db.Dialect == persist.Postgres {
		  res, err = q.ExecContext(ctx, `
DELETE FROM hmac_nonce_seen WHERE seen_id IN (
  SELECT seen_id FROM hmac_nonce_seen WHERE expire_at < $1 LIMIT $2
)`, now, limit)
	   } else {
		  res, err = q.ExecContext(ctx, `
DELETE FROM hmac_nonce_seen WHERE expire_at < ? LIMIT ?`,
			   now, limit)
	  }
	  if err != nil {
		  return 0, err
	   }
	  n, _ := res.RowsAffected()
	   return int(n), nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// SQLDriftProofRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLDriftProofRepo 基于 persist.SQLDB 的 HmacWriteDriftProof 存储实现（只增）。
type SQLDriftProofRepo struct {
	db *persist.SQLDB
}

// NewSQLDriftProofRepo 构造 SQL 漂移证明仓储。
   func NewSQLDriftProofRepo(db *persist.SQLDB) *SQLDriftProofRepo {
	   return &SQLDriftProofRepo{db: db}
   }

  func (r *SQLDriftProofRepo) SaveProof(ctx context.Context, p *HmacWriteDriftProof) error {
	  if p == nil || p.ProofID == "" {
		   return ErrInvalidInput
	   }
	   q := r.db.Conn(ctx)
	   var err error
	   if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, `
INSERT INTO hmac_write_proofs
  (proof_id, lease_id, key_id, nonce, write_scope, reason, detail,
   attempted_at, halt_gen, digest)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (proof_id) DO NOTHING
`,
			  p.ProofID, p.LeaseID, p.KeyID, p.Nonce, p.WriteScope,
			   string(p.Reason),
			  p.Detail, p.AttemptedAt.UTC(), p.HaltGen, p.Digest,
		   )
	   } else {
		   _, err = q.ExecContext(ctx, `
INSERT IGNORE INTO hmac_write_proofs
  (proof_id, lease_id, key_id, nonce, write_scope, reason, detail,
   attempted_at, halt_gen, digest)
VALUES (?,?,?,?,?,?,?,?,?,?)
`,
			  p.ProofID, p.LeaseID, p.KeyID, p.Nonce, p.WriteScope,
			   string(p.Reason),
			   p.Detail, p.AttemptedAt.UTC(), p.HaltGen, p.Digest,
		  )
	  }
	   return err
   }

  func (r *SQLDriftProofRepo) GetProof(ctx context.Context, proofID string) (*HmacWriteDriftProof, error) {
	  q := r.db.Conn(ctx)
	   ph := "?"
	   if r.db.Dialect == persist.Postgres {
		  ph = "$1"
	   }
	   row := q.QueryRowContext(ctx, ` 
SELECT proof_id, lease_id, key_id, nonce, write_scope, reason, detail,
       attempted_at, halt_gen, digest
FROM hmac_write_proofs WHERE proof_id = `+ph, proofID)
	   return scanDriftProof(row)
   }

  func (r *SQLDriftProofRepo) ListByLeaseID(ctx context.Context, leaseID string, limit int) ([]*HmacWriteDriftProof, error) {
	   q := r.db.Conn(ctx)
	   var rows *sql.Rows
	   var err error
	  if r.db.Dialect == persist.Postgres {
		   rows, err = q.QueryContext(ctx, `
SELECT proof_id, lease_id, key_id, nonce, write_scope, reason, detail,
       attempted_at, halt_gen, digest
FROM hmac_write_proofs WHERE lease_id = $1 ORDER BY attempted_at DESC LIMIT $2`,
			   leaseID, limit)
	   } else {
		   rows, err = q.QueryContext(ctx, `
SELECT proof_id, lease_id, key_id, nonce, write_scope, reason, detail,
       attempted_at, halt_gen, digest
FROM hmac_write_proofs WHERE lease_id = ? ORDER BY attempted_at DESC LIMIT ?`,
			  leaseID, limit)
	  }
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanDriftProofs(rows)
   }

   func (r *SQLDriftProofRepo) ListByReason(ctx context.Context, reason DriftReason, limit int) ([]*HmacWriteDriftProof, error) {
	  q := r.db.Conn(ctx)
	   var rows *sql.Rows
	   var err error
	   if r.db.Dialect == persist.Postgres {
		  rows, err = q.QueryContext(ctx, `
SELECT proof_id, lease_id, key_id, nonce, write_scope, reason, detail,
       attempted_at, halt_gen, digest
FROM hmac_write_proofs WHERE reason = $1 ORDER BY attempted_at DESC LIMIT $2`,
			  string(reason), limit)
	  } else {
		   rows, err = q.QueryContext(ctx, `
SELECT proof_id, lease_id, key_id, nonce, write_scope, reason, detail,
       attempted_at, halt_gen, digest
FROM hmac_write_proofs WHERE reason = ? ORDER BY attempted_at DESC LIMIT ?`,
			   string(reason), limit)
	  }
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  return scanDriftProofs(rows)
  }

   func scanDriftProof(row *sql.Row) (*HmacWriteDriftProof, error) {
	  var p HmacWriteDriftProof
	   var reason string
	  err := row.Scan(
		   &p.ProofID,
		   &p.LeaseID, &p.KeyID, &p.Nonce, &p.WriteScope,
		   &reason, &p.Detail, &p.AttemptedAt, &p.HaltGen, &p.Digest,
	   )
	   if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrLeaseNotFound
	   }
	  if err != nil {
		  return nil, err
	  }
	   p.Reason = DriftReason(reason)
	   p.AttemptedAt = p.AttemptedAt.UTC()
	  return &p, nil
   }

  func scanDriftProofs(rows *sql.Rows) ([]*HmacWriteDriftProof, error) {
	   var out []*HmacWriteDriftProof
	   for rows.Next() {
		   var p HmacWriteDriftProof
		   var reason string
		   if err := rows.Scan(
			   &p.ProofID, &p.LeaseID, &p.KeyID, &p.Nonce, &p.WriteScope,
			  &reason,
			  &p.Detail, &p.AttemptedAt, &p.HaltGen, &p.Digest,
		   ); err != nil {
			   return nil, err
		  }
		   p.Reason = DriftReason(reason)
		   p.AttemptedAt = p.AttemptedAt.UTC()
		  out = append(out, &p)
	   }
	  return out, rows.Err()
   }

// ──────────────────────────────────────────────────────────────────────────────
// SQLHaltGenRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLHaltGenRepo 基于 persist.SQLDB 的 HaltGeneration 存储实现。
type SQLHaltGenRepo struct {
	db *persist.SQLDB
}

// NewSQLHaltGenRepo 构造 SQL HaltGeneration 仓储。
   func NewSQLHaltGenRepo(db *persist.SQLDB) *SQLHaltGenRepo {
	   return &SQLHaltGenRepo{db: db}
   }

   func (r *SQLHaltGenRepo) SaveHaltGen(ctx context.Context, g *HaltGeneration) error {
	   if g == nil {
		   return ErrInvalidInput
	   }
	  q := r.db.Conn(ctx)
	  var err error
	   halted := 0
	   if g.Halted {
		   halted = 1
	   }
	  if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, `
INSERT INTO hmac_halt_generations (gen_id, generation, halted, updated_at)
VALUES ($1,$2,$3,$4)
ON CONFLICT (gen_id) DO UPDATE SET
  generation=EXCLUDED.generation,
  halted=EXCLUDED.halted,
  updated_at=EXCLUDED.updated_at
`,
			   g.GenID, g.Generation, halted, g.UpdatedAt.UTC(),
		  )
	  } else {
		  _, err = q.ExecContext(ctx, `
INSERT INTO hmac_halt_generations (gen_id, generation, halted, updated_at)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE
  generation=VALUES(generation),
  halted=VALUES(halted),
  updated_at=VALUES(updated_at)
`,
			  g.GenID, g.Generation, halted, g.UpdatedAt.UTC(),
		   )
	   }
	  return err
   }

   func (r *SQLHaltGenRepo) GetHaltGen(ctx context.Context) (*HaltGeneration, error) {
	  q := r.db.Conn(ctx)
	   ph := "?"
	   if r.db.Dialect == persist.Postgres {
		   ph = "$1"
	   }
	   row := q.QueryRowContext(ctx, ` 
SELECT gen_id, generation, halted, updated_at
FROM hmac_halt_generations WHERE gen_id = `+ph, HaltGenGlobalID)
	  var g HaltGeneration
	  var halted int
	   err := row.Scan(&g.GenID, &g.Generation, &halted, &g.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return &HaltGeneration{
			   GenID:      HaltGenGlobalID,
			   Generation: 0,
			  Halted: false,
		   }, nil
	  }
	  if err != nil {
		   return nil, err
	  }
	   g.Halted = halted != 0
	   g.UpdatedAt = g.UpdatedAt.UTC()
	  return &g, nil
  }

// ──────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────────────────────────────────────

   func nullableTimePtr(t *time.Time) sql.NullTime {
	   if t == nil {
		  return sql.NullTime{}
	   }
	  return sql.NullTime{Time: t.UTC(), Valid: true}
   }

   func itoa(n int) string {
	   buf := make([]byte, 0, 4)
	  if n == 0 {
		   return "0"
	  }
	  for n > 0 {
		   buf = append([]byte{byte('0' + n%10)}, buf...)
		   n /= 10
	   }
	  return string(buf)
  }
