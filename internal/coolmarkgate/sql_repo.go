package coolmarkgate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ──────────────────────────────────────────────────────────────────────────────
// SQLCoolLeaseRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLCoolLeaseRepo 基于 persist.SQLDB 的冷却租约存储实现。

type SQLCoolLeaseRepo struct {
	db *persist.SQLDB
}

// NewSQLCoolLeaseRepo 构造 SQL 冷却租约仓储。

   func NewSQLCoolLeaseRepo(db *persist.SQLDB) *SQLCoolLeaseRepo {
	  return &SQLCoolLeaseRepo{db: db}
   }

   func (r *SQLCoolLeaseRepo) SaveLease(ctx context.Context, l *CoolLease) error {
	   if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	   }
	   q := r.db.Conn(ctx)
	  var err error
	  if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO sweep_cool_leases
  (lease_id, asset, generation, state, marked_at, cool_until,
   cleared_at, proof_id, reason)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (lease_id) DO UPDATE SET
  state=EXCLUDED.state,
  cleared_at=EXCLUDED.cleared_at,
  proof_id=EXCLUDED.proof_id
`,
			  l.LeaseID,
			   l.Asset, l.Generation, string(l.State),
			  l.MarkedAt.UTC(), l.CoolUntil.UTC(),
			  nullableTimePtr(l.ClearedAt), l.ProofID, l.Reason,
		  )
	  } else {
		  _, err = q.ExecContext(ctx, `
INSERT INTO sweep_cool_leases
  (lease_id, asset, generation, state, marked_at, cool_until,
   cleared_at, proof_id, reason)
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state),
  cleared_at=VALUES(cleared_at),
  proof_id=VALUES(proof_id)
`,
			  l.LeaseID, l.Asset, l.Generation, string(l.State),
			  l.MarkedAt.UTC(), l.CoolUntil.UTC(),
			   nullableTimePtr(l.ClearedAt), l.ProofID, l.Reason,
		   )
	   }
	   return err
  }

  func (r *SQLCoolLeaseRepo) GetLease(ctx context.Context, leaseID string) (*CoolLease, error) {
	   q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `
SELECT lease_id, asset, generation, state, marked_at, cool_until,
       cleared_at, proof_id, reason
FROM sweep_cool_leases WHERE lease_id = `+r.ph(1),
		   leaseID,
	   )
	   return scanCoolLease(row)
   }

  func (r *SQLCoolLeaseRepo) GetActiveLeaseByAsset(ctx context.Context, asset string) (*CoolLease, error) {
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT lease_id, asset, generation, state, marked_at, cool_until,
       cleared_at, proof_id, reason
FROM sweep_cool_leases
WHERE asset = `+r.ph(1)+` AND state != 'cleared'
ORDER BY marked_at DESC LIMIT 1`,
		   strings.ToUpper(strings.TrimSpace(asset)),
	   )
	   return scanCoolLease(row)
   }

   func (r *SQLCoolLeaseRepo) ListActiveLeases(ctx context.Context, limit int) ([]*CoolLease, error) {
	  q := r.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		  `SELECT lease_id, asset, generation, state, marked_at, cool_until, 
		        cleared_at, proof_id, reason
		 FROM sweep_cool_leases WHERE state != 'cleared'
		 ORDER BY marked_at DESC LIMIT `+intStr(limit))
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanCoolLeaseRows(rows)
  }

  func (r *SQLCoolLeaseRepo) ListExpiredUncleared(ctx context.Context, before time.Time, limit int) ([]*CoolLease, error) {
	   q := r.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		   `SELECT lease_id, asset, generation, state, marked_at, cool_until,
		        cleared_at, proof_id, reason
		 FROM sweep_cool_leases
		 WHERE cool_until <= `+r.ph(1)+` AND state != 'cleared'
		 ORDER BY cool_until ASC LIMIT `+intStr(limit),
		   before.UTC(),
	   )
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanCoolLeaseRows(rows)
   }

  func (r *SQLCoolLeaseRepo) ph(n int) string {
	  if r.db.Dialect == persist.Postgres {
		  return pgPH(n)
	  }
	  return "?"
  }

// ──────────────────────────────────────────────────────────────────────────────
// SQLBreachCaseRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLBreachCaseRepo 基于 persist.SQLDB 的违约案例存储实现。
type SQLBreachCaseRepo struct {
	db *persist.SQLDB
}

// NewSQLBreachCaseRepo 构造 SQL 违约案例仓储。

  func NewSQLBreachCaseRepo(db *persist.SQLDB) *SQLBreachCaseRepo {
	   return &SQLBreachCaseRepo{db: db}
  }

  func (r *SQLBreachCaseRepo) SaveCase(ctx context.Context, c *CoolBreachCase) error {
	   if c == nil || c.CaseID == "" {
		   return ErrInvalidInput
	  }
	  q := r.db.Conn(ctx)
	   var err error
	   if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, `
INSERT INTO sweep_cool_cases
  (case_id, asset, generation, lease_id, exec_id, attempted_at,
   decision, reason, resolved, resolved_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (case_id) DO UPDATE SET
  resolved=EXCLUDED.resolved,
  resolved_at=EXCLUDED.resolved_at
`,
			  c.CaseID, c.Asset, c.Generation, c.LeaseID, c.ExecID,
			  c.AttemptedAt.UTC(), string(c.Decision), c.Reason,
			   c.Resolved, nullableTimePtr(c.ResolvedAt),
		   )
	   } else {
		  _, err = q.ExecContext(ctx, `
INSERT INTO sweep_cool_cases
  (case_id, asset, generation, lease_id, exec_id, attempted_at,
   decision, reason, resolved, resolved_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  resolved=VALUES(resolved),
  resolved_at=VALUES(resolved_at)
`,
			   c.CaseID, c.Asset, c.Generation, c.LeaseID, c.ExecID,
			  c.AttemptedAt.UTC(),
			  string(c.Decision), c.Reason,
			  c.Resolved, nullableTimePtr(c.ResolvedAt),
		   )
	  }
	   return err
  }

   func (r *SQLBreachCaseRepo) GetCase(ctx context.Context, caseID string) (*CoolBreachCase, error) {
	  q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, ` 
SELECT case_id, asset, generation, lease_id, exec_id, attempted_at,
       decision, reason, resolved, resolved_at
FROM sweep_cool_cases WHERE case_id = `+r.ph(1),
		  caseID,
	   )
	  return scanBreachCase(row)
  }

  func (r *SQLBreachCaseRepo) ListByAsset(ctx context.Context, asset string, limit int) ([]*CoolBreachCase, error) {
	   q := r.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, `
SELECT case_id, asset, generation, lease_id, exec_id, attempted_at,
       decision, reason, resolved, resolved_at
FROM sweep_cool_cases WHERE asset = `+r.ph(1)+`
ORDER BY attempted_at DESC LIMIT `+intStr(limit),
		   strings.ToUpper(strings.TrimSpace(asset)),
	   )
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanBreachCaseRows(rows)
  }

   func (r *SQLBreachCaseRepo) ListUnresolved(ctx context.Context, limit int) ([]*CoolBreachCase, error) {
	  q := r.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, `
SELECT case_id, asset, generation, lease_id, exec_id, attempted_at,
       decision, reason, resolved, resolved_at
FROM sweep_cool_cases WHERE resolved = false
ORDER BY attempted_at ASC LIMIT `+intStr(limit))
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanBreachCaseRows(rows)
   }

   func (r *SQLBreachCaseRepo) ph(n int) string {
	   if r.db.Dialect == persist.Postgres {
		  return pgPH(n)
	  }
	  return "?"
   }

// ──────────────────────────────────────────────────────────────────────────────
// SQLCoolProofRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLCoolProofRepo 基于 persist.SQLDB 的冷却证明存储实现。
type SQLCoolProofRepo struct {
	db *persist.SQLDB
}

// NewSQLCoolProofRepo 构造 SQL 冷却证明仓储。

  func NewSQLCoolProofRepo(db *persist.SQLDB) *SQLCoolProofRepo {
	  return &SQLCoolProofRepo{db: db}
   }

  func (r *SQLCoolProofRepo) SaveProof(ctx context.Context, p *CoolProof) error {
	  if p == nil || p.ProofID == "" {
		   return ErrInvalidInput
	   }
	  q := r.db.Conn(ctx)
	  var err error
	   if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, `
INSERT INTO sweep_cool_proofs
  (proof_id, lease_id, asset, generation, cool_until, signed_at, digest)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (proof_id) DO NOTHING
`,
			   p.ProofID, p.LeaseID, p.Asset, p.Generation,
			   p.CoolUntil.UTC(),
			  p.SignedAt.UTC(), p.Digest,
		  )
	  } else {
		  _, err = q.ExecContext(ctx, ` 
INSERT IGNORE INTO sweep_cool_proofs
  (proof_id, lease_id, asset, generation, cool_until, signed_at, digest)
VALUES (?,?,?,?,?,?,?)
`,
			  p.ProofID, p.LeaseID, p.Asset, p.Generation,
			  p.CoolUntil.UTC(), p.SignedAt.UTC(), p.Digest,
		   )
	   }
	  return err
   }

  func (r *SQLCoolProofRepo) GetProof(ctx context.Context, proofID string) (*CoolProof, error) {
	   q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, ` 
SELECT proof_id, lease_id, asset, generation, cool_until, signed_at, digest
FROM sweep_cool_proofs WHERE proof_id = `+r.ph(1),
		   proofID,
	  )
	   return scanCoolProof(row)
   }

  func (r *SQLCoolProofRepo) GetProofByLease(ctx context.Context, leaseID string) (*CoolProof, error) {
	   q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `
SELECT proof_id, lease_id, asset, generation, cool_until, signed_at, digest
FROM sweep_cool_proofs WHERE lease_id = `+r.ph(1)+` ORDER BY signed_at DESC LIMIT 1`,
		  leaseID,
	   )
	  return scanCoolProof(row)
   }

   func (r *SQLCoolProofRepo) GetProofByAssetGen(ctx context.Context, asset string, generation int64) (*CoolProof, error) {
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT proof_id, lease_id, asset, generation, cool_until, signed_at, digest
FROM sweep_cool_proofs
WHERE asset = `+r.ph(1)+` AND generation = `+r.ph(2)+`
ORDER BY signed_at DESC LIMIT 1`,
		  strings.ToUpper(strings.TrimSpace(asset)), generation,
	   )
	  return scanCoolProof(row)
   }

   func (r *SQLCoolProofRepo) ListByAsset(ctx context.Context, asset string, limit int) ([]*CoolProof, error) {
	  q := r.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, `
SELECT proof_id, lease_id, asset, generation, cool_until, signed_at, digest
FROM sweep_cool_proofs WHERE asset = `+r.ph(1)+`
ORDER BY signed_at DESC LIMIT `+intStr(limit),
		   strings.ToUpper(strings.TrimSpace(asset)),
	  )
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanCoolProofRows(rows)
  }

   func (r *SQLCoolProofRepo) ph(n int) string {
	   if r.db.Dialect == persist.Postgres {
		   return pgPH(n)
	   }
	   return "?"
  }

// ──────────────────────────────────────────────────────────────────────────────
// SQLCoolGenRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLCoolGenRepo 基于 persist.SQLDB 的世代标记存储实现。

type SQLCoolGenRepo struct {
	db *persist.SQLDB
}

// NewSQLCoolGenRepo 构造 SQL 世代仓储。

   func NewSQLCoolGenRepo(db *persist.SQLDB) *SQLCoolGenRepo {
	  return &SQLCoolGenRepo{db: db}
   }

   func (r *SQLCoolGenRepo) SaveGen(ctx context.Context, g *CoolGeneration) error {
	   if g == nil || g.Asset == "" {
		   return ErrInvalidInput
	  }
	  q := r.db.Conn(ctx)
	  var err error
	  if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO sweep_cool_generations
  (gen_id, asset, generation, active_lease_id, updated_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (asset) DO UPDATE SET
  generation=EXCLUDED.generation,
  active_lease_id=EXCLUDED.active_lease_id,
  updated_at=EXCLUDED.updated_at
`,
			   g.GenID, g.Asset, g.Generation, g.ActiveLeaseID, g.UpdatedAt.UTC(),
		  )
	   } else {
		  _, err = q.ExecContext(ctx, `
INSERT INTO sweep_cool_generations
  (gen_id, asset, generation, active_lease_id, updated_at)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  generation=VALUES(generation),
  active_lease_id=VALUES(active_lease_id),
  updated_at=VALUES(updated_at)
`,
			   g.GenID, g.Asset, g.Generation, g.ActiveLeaseID, g.UpdatedAt.UTC(),
		   )
	  }
	   return err
  }

  func (r *SQLCoolGenRepo) GetGen(ctx context.Context, asset string) (*CoolGeneration, error) {
	   q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT gen_id, asset, generation, active_lease_id, updated_at
FROM sweep_cool_generations WHERE asset = `+r.ph(1),
		   strings.ToUpper(strings.TrimSpace(asset)),
	  )
	   var g CoolGeneration
	  var updatedAt time.Time
	  err := row.Scan(&g.GenID, &g.Asset, &g.Generation, &g.ActiveLeaseID, &updatedAt)
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrCoolLeaseNotFound
		   }
		   return nil, err
	  }
	   g.UpdatedAt = updatedAt.UTC()
	   return &g, nil
   }

  func (r *SQLCoolGenRepo) ph(n int) string {
	  if r.db.Dialect == persist.Postgres {
		  return pgPH(n)
	  }
	  return "?"
  }

// ──────────────────────────────────────────────────────────────────────────────
// 扫描辅助
// ──────────────────────────────────────────────────────────────────────────────

type scannerRow interface {
	Scan(dest ...any) error
}

   func scanCoolLease(row scannerRow) (*CoolLease, error) {
	   var l CoolLease
	   var clearedAt sql.NullTime
	  var proofID, reason sql.NullString
	   err := row.Scan(&l.LeaseID, &l.Asset, &l.Generation, &l.State,
		  &l.MarkedAt, &l.CoolUntil, &clearedAt, &proofID, &reason)
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrCoolLeaseNotFound
		   }
		   return nil, err
	   }
	  l.MarkedAt = l.MarkedAt.UTC()
	   l.CoolUntil = l.CoolUntil.UTC()
	   if clearedAt.Valid {
		   t := clearedAt.Time.UTC()
		   l.ClearedAt = &t
	   }
	   if proofID.Valid {
		   l.ProofID = proofID.String
	   }
	  if reason.Valid {
		   l.Reason = reason.String
	   }
	  return &l, nil
   }

  func scanCoolLeaseRows(rows *sql.Rows) ([]*CoolLease, error) {
	   var out []*CoolLease
	  for rows.Next() {
		  l, err := scanCoolLease(rows)
		   if err != nil {
			  return out, err
		   }
		   out = append(out, l)
	  }
	  return out, rows.Err()
  }

  func scanBreachCase(row scannerRow) (*CoolBreachCase, error) {
	   var c CoolBreachCase
	   var resolvedAt sql.NullTime
	  var execID sql.NullString
	   err := row.Scan(&c.CaseID, &c.Asset, &c.Generation, &c.LeaseID, &execID,
		   &c.AttemptedAt, &c.Decision, &c.Reason, &c.Resolved, &resolvedAt)
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil,
				  ErrBreachCaseNotFound
		   }
		   return nil, err
	  }
	   c.AttemptedAt = c.AttemptedAt.UTC()
	  if execID.Valid {
		  c.ExecID = execID.String
	   }
	  if resolvedAt.Valid {
		  t := resolvedAt.Time.UTC()
		  c.ResolvedAt = &t
	   }
	   return &c, nil
   }

  func scanBreachCaseRows(rows *sql.Rows) ([]*CoolBreachCase, error) {
	   var out []*CoolBreachCase
	   for rows.Next() {
		   c, err := scanBreachCase(rows)
		   if err != nil {
			  return out, err
		   }
		   out = append(out, c)
	   }
	   return out, rows.Err()
   }

  func scanCoolProof(row scannerRow) (*CoolProof, error) {
	  var p CoolProof
	   err := row.Scan(&p.ProofID, &p.LeaseID, &p.Asset, &p.Generation,
		   &p.CoolUntil, &p.SignedAt, &p.Digest)
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrProofNotFound
		  }
		  return nil, err
	  }
	  p.CoolUntil = p.CoolUntil.UTC()
	   p.SignedAt = p.SignedAt.UTC()
	   return &p, nil
   }

  func scanCoolProofRows(rows *sql.Rows) ([]*CoolProof, error) {
	   var out []*CoolProof
	  for rows.Next() {
		   p, err := scanCoolProof(rows)
		  if err != nil {
			   return out, err
		   }
		   out = append(out, p)
	   }
	  return out, rows.Err()
   }

// ──────────────────────────────────────────────────────────────────────────────
// 工具函数
// ──────────────────────────────────────────────────────────────────────────────

   func nullableTimePtr(t *time.Time) sql.NullTime {
	   if t == nil {
		  return sql.NullTime{}
	   }
	  return sql.NullTime{Time: t.UTC(), Valid: true}
  }

  func pgPH(n int) string {
	   return "$" + intStr(n)
   }

  func intStr(n int) string {
	   if n <= 0 {
		  return "100"
	  }
	  b := make([]byte, 0, 4)
	   if n == 0 {
		  return "0"
	  }
	   tmp := n
	   digits := 0
	  for tmp > 0 {
		  digits++
		  tmp /= 10
	   }
	  b = make([]byte, digits)
	   for i := digits - 1; i >= 0; i-- {
		   b[i] = byte('0' + n%10)
		  n /= 10
	   }
	   return string(b)
  }
