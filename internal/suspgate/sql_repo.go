package suspgate

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ──────────────────────────────────────────────────────────────────────────────
// 占位符辅助
// ──────────────────────────────────────────────────────────────────────────────

   func sgPH(db *persist.SQLDB, n int) string {
	   if db != nil && db.Dialect == persist.Postgres {
		  return "$" + strconv.Itoa(n)
	  }
	  return "?"
  }

// ──────────────────────────────────────────────────────────────────────────────
// SQLGenCounterRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLGenCounterRepo 基于 persist.SQLDB 的世代计数器仓储实现。
type SQLGenCounterRepo struct {
	db *persist.SQLDB
}

// NewSQLGenCounterRepo 构造 SQL 世代计数器仓储。
   func NewSQLGenCounterRepo(db *persist.SQLDB) *SQLGenCounterRepo {
	   return &SQLGenCounterRepo{db: db}
   }

   func (r *SQLGenCounterRepo) ph(n int) string { return sgPH(r.db, n) }

  func (r *SQLGenCounterRepo) GetCounter(ctx context.Context, userID, asset string) (*GenCounter, error) {
	  key := GenCounterKey(userID, asset)
	  q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx,
		   `SELECT counter_key, user_id, asset, current_gen, active_count, updated_at 
		 FROM suspense_gen_counters WHERE counter_key = `+r.ph(1), key)
	  return scanGenCounter(row)
   }

   func (r *SQLGenCounterRepo) IncrAndGet(ctx context.Context, userID, asset string, prevGen int64) (*GenCounter, error) {
	  key := GenCounterKey(userID, asset)
	   uid := strings.TrimSpace(userID)
	  ast := strings.ToUpper(strings.TrimSpace(asset))
	   now := time.Now().UTC()
	  q := r.db.Conn(ctx)
	   if r.db.Dialect == persist.Postgres {
		  var newGen int64
		  var activeCount int
		   err := q.QueryRowContext(ctx, ` 
INSERT INTO suspense_gen_counters (counter_key, user_id, asset, current_gen, active_count, updated_at)
VALUES ($1, $2, $3, 1, 1, $4)
ON CONFLICT (counter_key) DO UPDATE
  SET current_gen  = CASE WHEN suspense_gen_counters.current_gen = $5
                          THEN suspense_gen_counters.current_gen + 1
                          ELSE suspense_gen_counters.current_gen END,
      active_count = CASE WHEN suspense_gen_counters.current_gen = $5
                          THEN suspense_gen_counters.active_count + 1
                          ELSE suspense_gen_counters.active_count END,
      updated_at   = CASE WHEN suspense_gen_counters.current_gen = $5
                          THEN $4 ELSE suspense_gen_counters.updated_at END
RETURNING current_gen, active_count
`,
			  key, uid, ast, now, prevGen,
		   ).Scan(&newGen, &activeCount)
		   if err != nil {
			   return nil, err
		   }
		  if newGen == prevGen {
			   return nil, ErrGenConflict
		   }
		   return &GenCounter{
			  CounterKey: key,
			  UserID:     uid,
			  Asset: ast,
			  CurrentGen: newGen,
			   ActiveCount: activeCount,
			  UpdatedAt:   now,
		   }, nil
	   }
	   // MySQL 路径：INSERT … ON DUPLICATE KEY UPDATE + 二次读。
	   _, err := q.ExecContext(ctx, ` 
INSERT INTO suspense_gen_counters (counter_key, user_id, asset, current_gen, active_count, updated_at)
VALUES (?, ?, ?, 1, 1, ?)
ON DUPLICATE KEY UPDATE
  current_gen  = IF(current_gen = ?, current_gen + 1, current_gen),
  active_count = IF(current_gen = ?, active_count + 1, active_count),
  updated_at   = IF(current_gen = ?, ?, updated_at)
`,
		  key, uid, ast, now,
		  prevGen, prevGen, prevGen, now,
	   )
	  if err != nil {
		  return nil, err
	  }
	  c, err := r.GetCounter(ctx, userID, asset)
	  if err != nil {
		   return nil, err
	  }
	  if c.CurrentGen == prevGen {
		   return nil, ErrGenConflict
	   }
	   return c, nil
   }

   func (r *SQLGenCounterRepo) IncrActiveCount(ctx context.Context, userID, asset string) error {
	   key := GenCounterKey(userID,
		   asset)
	   now := time.Now().UTC()
	  q := r.db.Conn(ctx)
	  _, err := q.ExecContext(ctx,
		  `UPDATE suspense_gen_counters SET active_count = active_count + 1, updated_at = ?
		 WHERE counter_key = ?`, now, key)
	   return err
   }

  func (r *SQLGenCounterRepo) DecrActiveCount(ctx context.Context, userID, asset string) (int, error) {
	  key := GenCounterKey(userID,
		   asset)
	   now := time.Now().UTC()
	  q := r.db.Conn(ctx)
	  _, err := q.ExecContext(ctx,
		  `UPDATE suspense_gen_counters
		 SET active_count = CASE WHEN active_count > 0 THEN active_count - 1 ELSE 0 END,
		     updated_at = ?
		 WHERE counter_key = ?`, now, key)
	   if err != nil {
		   return 0, err
	  }
	  c, err := r.GetCounter(ctx, userID, asset)
	   if err != nil {
		   return 0, err
	   }
	  return c.ActiveCount, nil
  }

  func (r *SQLGenCounterRepo) SaveCounter(ctx context.Context, c *GenCounter) error {
	  if c == nil || c.CounterKey == "" {
		   return ErrInvalidInput
	   }
	   q := r.db.Conn(ctx)
	   now := time.Now().UTC()
	   if r.db.Dialect == persist.Postgres {
		  _, err := q.ExecContext(ctx, `
INSERT INTO suspense_gen_counters (counter_key, user_id, asset, current_gen, active_count, updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (counter_key) DO UPDATE SET
  current_gen  = EXCLUDED.current_gen,
  active_count = EXCLUDED.active_count,
  updated_at   = EXCLUDED.updated_at
`, c.CounterKey, c.UserID, c.Asset, c.CurrentGen, c.ActiveCount, now)
		  return err
	   }
	   _, err := q.ExecContext(ctx, `
INSERT INTO suspense_gen_counters (counter_key, user_id, asset, current_gen, active_count, updated_at)
VALUES (?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  current_gen  = VALUES(current_gen),
  active_count = VALUES(active_count),
  updated_at   = VALUES(updated_at)
`, c.CounterKey, c.UserID, c.Asset, c.CurrentGen, c.ActiveCount, now)
	  return err
  }

   func scanGenCounter(row *sql.Row) (*GenCounter, error) {
	   var c GenCounter
	   var updAt time.Time
	  if err := row.Scan(&c.CounterKey, &c.UserID, &c.Asset, &c.CurrentGen, &c.ActiveCount, &updAt); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrGenNotFound
		  }
		   return nil, err
	   }
	   c.UpdatedAt = updAt.UTC()
	   return &c, nil
  }

// ──────────────────────────────────────────────────────────────────────────────
// SQLGenRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLGenRepo 基于 persist.SQLDB 的暂停世代仓储实现。

type SQLGenRepo struct {
	db *persist.SQLDB
}

// NewSQLGenRepo 构造 SQL 暂停世代仓储。
  func NewSQLGenRepo(db *persist.SQLDB) *SQLGenRepo {
	  return &SQLGenRepo{db: db}
  }

   func (r *SQLGenRepo) GetGen(ctx context.Context, genID string) (*SuspenseGeneration, error) {
	   q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		   `SELECT gen_id, user_id, asset, gen_value, case_id, suspended_by, state,
		        clear_actor1, clear_actor2, clear_note, suspended_at, cleared_at, updated_at
		 FROM suspense_gens WHERE gen_id = ?`, genID)
	  return scanGen(row)
   }

   func (r *SQLGenRepo) GetActiveGen(ctx context.Context, userID, asset string) (*SuspenseGeneration, error) {
	   norm := strings.ToUpper(strings.TrimSpace(asset))
	  uid := strings.TrimSpace(userID)
	   q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx,
		   `SELECT gen_id, user_id, asset, gen_value, case_id, suspended_by, state,
		        clear_actor1, clear_actor2, clear_note, suspended_at, cleared_at, updated_at
		 FROM suspense_gens
		 WHERE user_id = ? AND asset = ? AND state IN ('active','clearing')
		 ORDER BY gen_value DESC LIMIT 1`,
		   uid, norm)
	  return scanGen(row)
   }

   func (r *SQLGenRepo) ListActiveGens(ctx context.Context, userID, asset string) ([]*SuspenseGeneration, error) {
	  norm := strings.ToUpper(strings.TrimSpace(asset))
	   uid := strings.TrimSpace(userID)
	  q := r.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		   `SELECT gen_id, user_id, asset, gen_value, case_id, suspended_by, state,
		        clear_actor1, clear_actor2, clear_note, suspended_at, cleared_at, updated_at
		 FROM suspense_gens
		 WHERE user_id = ? AND asset = ? AND state IN ('active','clearing')
		 ORDER BY gen_value`,
		  uid, norm)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  return scanGenRows(rows)
  }

  func (r *SQLGenRepo) SaveGen(ctx context.Context, g *SuspenseGeneration) error {
	   if g == nil || g.GenID == "" {
		   return ErrInvalidInput
	   }
	   q := r.db.Conn(ctx)
	   now := time.Now().UTC()
	  if r.db.Dialect == persist.Postgres {
		  _, err := q.ExecContext(ctx, `
INSERT INTO suspense_gens
  (gen_id, user_id, asset, gen_value, case_id, suspended_by, state,
   clear_actor1, clear_actor2, clear_note, suspended_at, cleared_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (gen_id) DO UPDATE SET
  state        = EXCLUDED.state,
  clear_actor1 = EXCLUDED.clear_actor1,
  clear_actor2 = EXCLUDED.clear_actor2,
  clear_note   = EXCLUDED.clear_note,
  cleared_at   = EXCLUDED.cleared_at,
  updated_at   = EXCLUDED.updated_at
`,
			  g.GenID, g.UserID, g.Asset, g.GenValue, g.CaseID, g.SuspendedBy, string(g.State),
			  g.ClearActor1, g.ClearActor2, g.ClearNote,
			   g.SuspendedAt.UTC(), nullableTimePtr(g.ClearedAt), now,
		   )
		  return err
	  }
	   _, err := q.ExecContext(ctx, `
INSERT INTO suspense_gens
  (gen_id, user_id, asset, gen_value, case_id, suspended_by, state,
   clear_actor1, clear_actor2, clear_note, suspended_at, cleared_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state        = VALUES(state),
  clear_actor1 = VALUES(clear_actor1),
  clear_actor2 = VALUES(clear_actor2),
  clear_note   = VALUES(clear_note),
  cleared_at   = VALUES(cleared_at),
  updated_at   = VALUES(updated_at)
`,
		  g.GenID, g.UserID, g.Asset, g.GenValue, g.CaseID, g.SuspendedBy, string(g.State),
		   g.ClearActor1, g.ClearActor2, g.ClearNote,
		   g.SuspendedAt.UTC(), nullableTimePtr(g.ClearedAt), now,
	   )
	  return err
  }

   func (r *SQLGenRepo) SaveGenCleared(ctx context.Context, g *SuspenseGeneration, prevState SuspendedState) error {
	   if g == nil || g.GenID == "" {
		   return ErrInvalidInput
	  }
	   if g.ClearedAt == nil {
		   return ErrInvalidInput
	  }
	  q := r.db.Conn(ctx)
	   now := time.Now().UTC()
	  res, err := q.ExecContext(ctx,
		   `UPDATE suspense_gens
		 SET state = ?, clear_actor1 = ?, clear_actor2 = ?,
		     clear_note = ?, cleared_at = ?, updated_at = ?
		 WHERE gen_id = ? AND state = ?`,
		   string(SuspCleared),
		  g.ClearActor1, g.ClearActor2, g.ClearNote,
		  g.ClearedAt.UTC(), now, g.GenID, string(prevState),
	  )
	   if err != nil {
		  return err
	   }
	  n, err := res.RowsAffected()
	   if err != nil {
		  return err
	   }
	   if n == 0 {
		  existing, gerr := r.GetGen(ctx, g.GenID)
		   if gerr != nil {
			   return ErrGenConflict
		  }
		  if existing.State == SuspCleared {
			   return ErrGenAlreadyCleared
		   }
		   return ErrGenConflict
	  }
	   return nil
   }

  func (r *SQLGenRepo) ListByState(ctx context.Context, state SuspendedState, limit int) ([]*SuspenseGeneration, error) {
	   if limit <= 0 {
		  limit = 100
	  }
	  q := r.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		  `SELECT gen_id, user_id, asset, gen_value, case_id, suspended_by, state,
		        clear_actor1, clear_actor2, clear_note, suspended_at, cleared_at, updated_at
		 FROM suspense_gens WHERE state = ? ORDER BY suspended_at LIMIT ?`,
		   string(state), limit)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanGenRows(rows)
  }

  func scanGen(row *sql.Row) (*SuspenseGeneration, error) {
	  var g SuspenseGeneration
	   var state string
	  var clearedAt sql.NullTime
	   var suspAt, updAt time.Time
	  err := row.Scan(
		  &g.GenID, &g.UserID, &g.Asset, &g.GenValue, &g.CaseID, &g.SuspendedBy, &state,
		   &g.ClearActor1, &g.ClearActor2, &g.ClearNote,
		   &suspAt, &clearedAt, &updAt,
	  )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrGenNotFound
		  }
		   return nil, err
	  }
	   g.State = SuspendedState(state)
	   g.SuspendedAt = suspAt.UTC()
	   g.UpdatedAt = updAt.UTC()
	  if clearedAt.Valid {
		   t := clearedAt.Time.UTC()
		  g.ClearedAt = &t
	   }
	  return &g, nil
  }

  func scanGenRows(rows *sql.Rows) ([]*SuspenseGeneration, error) {
	  var out []*SuspenseGeneration
	  for rows.Next() {
		   var g SuspenseGeneration
		  var state string
		   var clearedAt sql.NullTime
		   var suspAt, updAt time.Time
		  err := rows.Scan(
			  &g.GenID,
			   &g.UserID, &g.Asset, &g.GenValue, &g.CaseID, &g.SuspendedBy, &state,
			  &g.ClearActor1, &g.ClearActor2, &g.ClearNote,
			   &suspAt, &clearedAt, &updAt,
		   )
		  if err != nil {
			  return nil, err
		  }
		  g.State = SuspendedState(state)
		   g.SuspendedAt = suspAt.UTC()
		   g.UpdatedAt = updAt.UTC()
		  if clearedAt.Valid {
			  t := clearedAt.Time.UTC()
			   g.ClearedAt = &t
		  }
		  out = append(out, &g)
	  }
	   return out, rows.Err()
  }

// ──────────────────────────────────────────────────────────────────────────────
// SQLLeaseRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLLeaseRepo 基于 persist.SQLDB 的清障租约仓储实现。

type SQLLeaseRepo struct {
	db *persist.SQLDB
}

// NewSQLLeaseRepo 构造 SQL 清障租约仓储。
   func NewSQLLeaseRepo(db *persist.SQLDB) *SQLLeaseRepo {
	   return &SQLLeaseRepo{db: db}
   }

   func (r *SQLLeaseRepo) GetLease(ctx context.Context, leaseID string) (*ClearanceLease, error) {
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		   `SELECT lease_id, user_id, asset, gen_value, case_id, state, 
		        issued_at, expire_at, consumed_at, consumed_by
		 FROM clearance_leases WHERE lease_id = ?`, leaseID)
	   return scanLease(row)
   }

  func (r *SQLLeaseRepo) GetLeaseByGen(ctx context.Context, userID, asset string, genValue int64) (*ClearanceLease, error) {
	   norm := strings.ToUpper(strings.TrimSpace(asset))
	   uid := strings.TrimSpace(userID)
	  q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx,
		  `SELECT lease_id, user_id, asset, gen_value, case_id, state, 
		        issued_at, expire_at, consumed_at, consumed_by
		 FROM clearance_leases
		 WHERE user_id = ? AND asset = ? AND gen_value = ? AND state = 'issued'
		 ORDER BY issued_at DESC LIMIT 1`,
		   uid, norm, genValue)
	   return scanLease(row)
   }

   func (r *SQLLeaseRepo) SaveLease(ctx context.Context, l *ClearanceLease) error {
	  if l == nil || l.LeaseID == "" {
		  return ErrInvalidInput
	   }
	   q := r.db.Conn(ctx)
	  if r.db.Dialect == persist.Postgres {
		  _, err := q.ExecContext(ctx, `
INSERT INTO clearance_leases
  (lease_id, user_id, asset, gen_value, case_id, state, issued_at, expire_at, consumed_at, consumed_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (lease_id) DO UPDATE SET
  state       = EXCLUDED.state,
  consumed_at = EXCLUDED.consumed_at,
  consumed_by = EXCLUDED.consumed_by
`,
			  l.LeaseID, l.UserID, l.Asset, l.GenValue, l.CaseID, string(l.State),
			   l.IssuedAt.UTC(),
			  l.ExpireAt.UTC(),
			  nullableTimePtr(l.ConsumedAt), l.ConsumedBy,
		   )
		   return err
	  }
	  _, err := q.ExecContext(ctx, ` 
INSERT INTO clearance_leases
  (lease_id, user_id, asset, gen_value, case_id, state, issued_at, expire_at, consumed_at, consumed_by)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state       = VALUES(state),
  consumed_at = VALUES(consumed_at),
  consumed_by = VALUES(consumed_by)
`,
		   l.LeaseID,
		   l.UserID, l.Asset, l.GenValue, l.CaseID, string(l.State),
		  l.IssuedAt.UTC(), l.ExpireAt.UTC(),
		  nullableTimePtr(l.ConsumedAt), l.ConsumedBy,
	  )
	   return err
  }

  func (r *SQLLeaseRepo) ListByState(ctx context.Context, state LeaseState, limit int) ([]*ClearanceLease, error) {
	   if limit <= 0 {
		  limit = 100
	  }
	   q := r.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		   `SELECT lease_id, user_id, asset, gen_value, case_id, state,
		        issued_at, expire_at, consumed_at, consumed_by
		 FROM clearance_leases WHERE state = ? ORDER BY issued_at LIMIT ?`,
		   string(state), limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  var out []*ClearanceLease
	  for rows.Next() {
		   var l ClearanceLease
		   var stateStr string
		  var issuedAt, expireAt time.Time
		   var consumedAt sql.NullTime
		   if err := rows.Scan(&l.LeaseID, &l.UserID, &l.Asset, &l.GenValue, &l.CaseID, &stateStr,
			   &issuedAt, &expireAt, &consumedAt, &l.ConsumedBy); err != nil {
			   return nil, err
		   }
		   l.State = LeaseState(stateStr)
		  l.IssuedAt = issuedAt.UTC()
		   l.ExpireAt = expireAt.UTC()
		  if consumedAt.Valid {
			   t := consumedAt.Time.UTC()
			  l.ConsumedAt = &t
		  }
		  out = append(out, &l)
	  }
	   return out, rows.Err()
   }

   func scanLease(row *sql.Row) (*ClearanceLease, error) {
	  var l ClearanceLease
	   var stateStr string
	  var issuedAt, expireAt time.Time
	   var consumedAt sql.NullTime
	  err := row.Scan(
		  &l.LeaseID, &l.UserID, &l.Asset, &l.GenValue, &l.CaseID, &stateStr,
		  &issuedAt, &expireAt, &consumedAt, &l.ConsumedBy,
	  )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrLeaseNotFound
		   }
		   return nil, err
	  }
	   l.State = LeaseState(stateStr)
	   l.IssuedAt = issuedAt.UTC()
	   l.ExpireAt = expireAt.UTC()
	  if consumedAt.Valid {
		   t := consumedAt.Time.UTC()
		  l.ConsumedAt = &t
	  }
	  return &l, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// SQLDriftProofRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLDriftProofRepo 基于 persist.SQLDB 的漂移证明仓储实现（只增）。
type SQLDriftProofRepo struct {
	db *persist.SQLDB
}

// NewSQLDriftProofRepo 构造 SQL 漂移证明仓储。
  func NewSQLDriftProofRepo(db *persist.SQLDB) *SQLDriftProofRepo {
	   return &SQLDriftProofRepo{db: db}
  }

  func (r *SQLDriftProofRepo) SaveProof(ctx context.Context, p *SuspenseDriftProof) error {
	   if p == nil || p.ProofID == "" {
		  return ErrInvalidInput
	   }
	  q := r.db.Conn(ctx)
	   if r.db.Dialect == persist.Postgres {
		   _, err := q.ExecContext(ctx, `
INSERT INTO suspense_proofs
  (proof_id, case_id, user_id, asset, susp_gen, count_before, count_after,
   block_outcome, detail, proof_hash, attempted_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (proof_id) DO NOTHING
`,
			  p.ProofID, p.CaseID, p.UserID, p.Asset, p.SuspGen,
			  p.CountBefore,
			  p.CountAfter, string(p.BlockOutcome),
			  p.Detail,
			   p.ProofHash, p.AttemptedAt.UTC(),
		  )
		   return err
	   }
	  _, err := q.ExecContext(ctx, `
INSERT IGNORE INTO suspense_proofs
  (proof_id, case_id, user_id, asset, susp_gen, count_before, count_after,
   block_outcome, detail, proof_hash, attempted_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
`,
		   p.ProofID, p.CaseID, p.UserID, p.Asset, p.SuspGen,
		  p.CountBefore, p.CountAfter, string(p.BlockOutcome),
		  p.Detail, p.ProofHash, p.AttemptedAt.UTC(),
	  )
	  return err
  }

   func (r *SQLDriftProofRepo) GetProof(ctx context.Context, proofID string) (*SuspenseDriftProof, error) {
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		  `SELECT proof_id, case_id, user_id, asset, susp_gen, count_before, count_after,
		        block_outcome, detail, proof_hash, attempted_at
		 FROM suspense_proofs WHERE proof_id = ?`, proofID)
	   return scanDriftProof(row)
  }

  func (r *SQLDriftProofRepo) ListByCase(ctx context.Context, caseID string) ([]*SuspenseDriftProof, error) {
	  q := r.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		  `SELECT proof_id, case_id, user_id, asset, susp_gen, count_before, count_after,
		        block_outcome, detail, proof_hash, attempted_at
		 FROM suspense_proofs WHERE case_id = ? ORDER BY attempted_at`, caseID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanDriftProofRows(rows)
  }

  func (r *SQLDriftProofRepo) ListByOutcome(ctx context.Context, outcome ProofOutcome, limit int) ([]*SuspenseDriftProof, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  q := r.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		  `SELECT proof_id, case_id, user_id, asset, susp_gen, count_before, count_after,
		        block_outcome, detail, proof_hash, attempted_at
		 FROM suspense_proofs WHERE block_outcome = ? ORDER BY attempted_at DESC LIMIT ?`,
		   string(outcome), limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanDriftProofRows(rows)
  }

  func scanDriftProof(row *sql.Row) (*SuspenseDriftProof, error) {
	  var p SuspenseDriftProof
	   var outcome string
	   var attemptedAt time.Time
	  err := row.Scan(
		   &p.ProofID, &p.CaseID, &p.UserID, &p.Asset, &p.SuspGen,
		   &p.CountBefore,
		  &p.CountAfter, &outcome,
		   &p.Detail, &p.ProofHash, &attemptedAt,
	  )
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrGenNotFound
		   }
		  return nil, err
	   }
	  p.BlockOutcome = ProofOutcome(outcome)
	   p.AttemptedAt = attemptedAt.UTC()
	  return &p, nil
   }

  func scanDriftProofRows(rows *sql.Rows) ([]*SuspenseDriftProof, error) {
	  var out []*SuspenseDriftProof
	  for rows.Next() {
		  var p SuspenseDriftProof
		  var outcome string
		   var attemptedAt time.Time
		  if err := rows.Scan(
			   &p.ProofID, &p.CaseID, &p.UserID, &p.Asset, &p.SuspGen,
			  &p.CountBefore,
			   &p.CountAfter, &outcome,
			   &p.Detail, &p.ProofHash, &attemptedAt,
		  ); err != nil {
			  return nil, err
		  }
		   p.BlockOutcome = ProofOutcome(outcome)
		  p.AttemptedAt = attemptedAt.UTC()
		  out = append(out, &p)
	  }
	  return out, rows.Err()
  }

// nullableTimePtr 将 *time.Time 转为 sql.NullTime 供驱动使用。
   func nullableTimePtr(t *time.Time) sql.NullTime {
	  if t == nil {
		  return sql.NullTime{}
	   }
	   return sql.NullTime{Time: t.UTC(), Valid: true}
  }
