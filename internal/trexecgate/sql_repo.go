package trexecgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ──────────────────────────────────────────────────────────────────────────────
// SQLLeaseRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLLeaseRepo 基于 persist.SQLDB 的租约存储实现。
type SQLLeaseRepo struct {
	db *persist.SQLDB
}

// NewSQLLeaseRepo 构造 SQL 租约仓储。
   func NewSQLLeaseRepo(db *persist.SQLDB) *SQLLeaseRepo {
	  return &SQLLeaseRepo{db: db}
   }

// SaveLease 新建或更新租约（upsert by lease_id）。
  func (r *SQLLeaseRepo) SaveLease(ctx context.Context, l *PlanExecLease) error {
	  if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	  }
	   snapJSON,
		  err := json.Marshal(l.SnapshotDigest)
	   if err != nil {
		   return err
	  }
	  recheckAt := nullableTime(l.RecheckAt)
	  sentAt := nullableTime(l.SentAt)
	   ackedAt := nullableTime(l.AckedAt)
	   blockedAt := nullableTime(l.BlockedAt)
	  q := r.db.Conn(ctx)
	  if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, ` 
INSERT INTO treasury_plan_leases
  (lease_id, idem_key, exec_id, asset, amount, snapshot_json,
   state, ttl_seconds, bound_at, expires_at,
   recheck_at, sent_at, acked_at, blocked_at, block_reason)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (lease_id) DO UPDATE SET
  exec_id=EXCLUDED.exec_id,
  snapshot_json=EXCLUDED.snapshot_json,
  state=EXCLUDED.state,
  ttl_seconds=EXCLUDED.ttl_seconds,
  expires_at=EXCLUDED.expires_at,
  recheck_at=EXCLUDED.recheck_at,
  sent_at=EXCLUDED.sent_at,
  acked_at=EXCLUDED.acked_at,
  blocked_at=EXCLUDED.blocked_at,
  block_reason=EXCLUDED.block_reason
`,
			   l.LeaseID,
			   l.IdemKey, l.ExecID, l.Asset, l.Amount, string(snapJSON),
			   string(l.State), int64(l.TTL.Seconds()),
			   l.BoundAt.UTC(), l.ExpiresAt.UTC(),
			  recheckAt, sentAt, ackedAt, blockedAt, l.BlockReason,
		  )
	  } else {
		   _, err = q.ExecContext(ctx, `
INSERT INTO treasury_plan_leases
  (lease_id, idem_key, exec_id, asset, amount, snapshot_json,
   state, ttl_seconds, bound_at, expires_at,
   recheck_at, sent_at, acked_at, blocked_at, block_reason)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  exec_id=VALUES(exec_id),
  snapshot_json=VALUES(snapshot_json),
  state=VALUES(state),
  ttl_seconds=VALUES(ttl_seconds),
  expires_at=VALUES(expires_at),
  recheck_at=VALUES(recheck_at),
  sent_at=VALUES(sent_at),
  acked_at=VALUES(acked_at),
  blocked_at=VALUES(blocked_at),
  block_reason=VALUES(block_reason)
`,
			   l.LeaseID, l.IdemKey, l.ExecID, l.Asset, l.Amount, string(snapJSON),
			  string(l.State), int64(l.TTL.Seconds()),
			   l.BoundAt.UTC(), l.ExpiresAt.UTC(),
			  recheckAt,
			   sentAt, ackedAt, blockedAt, l.BlockReason,
		  )
	  }
	   return err
  }

  func (r *SQLLeaseRepo) GetLease(ctx context.Context, leaseID string) (*PlanExecLease, error) {
	  row := r.db.Conn(ctx).QueryRowContext(ctx,
		  persist.Rewrite(`SELECT lease_id,idem_key,exec_id,asset,amount,snapshot_json, 
		   state,ttl_seconds,bound_at,expires_at,
		   recheck_at,sent_at,acked_at,blocked_at,block_reason
		FROM treasury_plan_leases WHERE lease_id=?`, r.db.Dialect),
		  strings.TrimSpace(leaseID),
	   )
	   return scanLease(row)
   }

  func (r *SQLLeaseRepo) GetLeaseByIdemKey(ctx context.Context, idemKey string) (*PlanExecLease, error) {
	  row := r.db.Conn(ctx).QueryRowContext(ctx,
		   persist.Rewrite(`SELECT lease_id,idem_key,exec_id,asset,amount,snapshot_json,
		   state,ttl_seconds,bound_at,expires_at,
		   recheck_at,sent_at,acked_at,blocked_at,block_reason
		FROM treasury_plan_leases
		WHERE idem_key=?
		ORDER BY bound_at DESC LIMIT 1`, r.db.Dialect),
		  strings.TrimSpace(idemKey),
	   )
	  return scanLease(row)
  }

   func (r *SQLLeaseRepo) GetLeaseByExecID(ctx context.Context, execID string) (*PlanExecLease, error) {
	   row := r.db.Conn(ctx).QueryRowContext(ctx,
		  persist.Rewrite(`SELECT lease_id,idem_key,exec_id,asset,amount,snapshot_json, 
		   state,ttl_seconds,bound_at,expires_at,
		   recheck_at,sent_at,acked_at,blocked_at,block_reason
		FROM treasury_plan_leases WHERE exec_id=? LIMIT 1`, r.db.Dialect),
		   strings.TrimSpace(execID),
	   )
	  return scanLease(row)
   }

  func (r *SQLLeaseRepo) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*PlanExecLease, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   persist.Rewrite(`SELECT lease_id,idem_key,exec_id,asset,amount,snapshot_json,
		   state,ttl_seconds,bound_at,expires_at,
		   recheck_at,sent_at,acked_at,blocked_at,block_reason
		FROM treasury_plan_leases
		WHERE expires_at <= ? AND state NOT IN ('acked','expired','blocked')
		ORDER BY expires_at ASC LIMIT ?`, r.db.Dialect),
		  before.UTC(), limit,
	   )
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanLeases(rows)
  }

   func (r *SQLLeaseRepo) ListByState(ctx context.Context, state LeaseState, limit int) ([]*PlanExecLease, error) {
	  if limit <= 0 {
		   limit = 50
	  }
	  rows,
		  err := r.db.Conn(ctx).QueryContext(ctx,
		  persist.Rewrite(`SELECT lease_id,idem_key,exec_id,asset,amount,snapshot_json,
		   state,ttl_seconds,bound_at,expires_at,
		   recheck_at,sent_at,acked_at,blocked_at,block_reason
		FROM treasury_plan_leases
		WHERE state=? ORDER BY bound_at ASC LIMIT ?`, r.db.Dialect),
		  string(state), limit,
	  )
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanLeases(rows)
   }

// ──────────────────────────────────────────────────────────────────────────────
// SQLDriftProofRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLDriftProofRepo 基于 persist.SQLDB 的漂移证明存储实现。
type SQLDriftProofRepo struct {
	db *persist.SQLDB
}

// NewSQLDriftProofRepo 构造 SQL 漂移证明仓储。
   func NewSQLDriftProofRepo(db *persist.SQLDB) *SQLDriftProofRepo {
	  return &SQLDriftProofRepo{db: db}
  }

  func (r *SQLDriftProofRepo) SaveProof(ctx context.Context, p *TreasDriftProof) error {
	   if p == nil || p.ProofID == "" {
		   return ErrInvalidInput
	   }
	  recJSON, _ := json.Marshal(p.RecordDigest)
	  recheckJSON, _ := json.Marshal(p.RecheckDigest)
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		   persist.Rewrite(`INSERT IGNORE INTO treasury_exec_drift_proofs
		(proof_id, lease_id, exec_id, asset,
		 record_snapshot_json, recheck_snapshot_json,
		 drift_abs, drift_ratio, decision, reason, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, r.db.Dialect),
		  p.ProofID,
		   p.LeaseID, p.ExecID, p.Asset,
		   string(recJSON), string(recheckJSON),
		   p.DriftAbs, p.DriftRatio, p.Decision, p.Reason,
		  p.CreatedAt.UTC(),
	   )
	  return err
  }

  func (r *SQLDriftProofRepo) GetProof(ctx context.Context, proofID string) (*TreasDriftProof, error) {
	  row := r.db.Conn(ctx).QueryRowContext(ctx,
		   persist.Rewrite(`SELECT proof_id,lease_id,exec_id,asset,
		   record_snapshot_json,recheck_snapshot_json,
		   drift_abs,drift_ratio,decision,reason,created_at
		FROM treasury_exec_drift_proofs WHERE proof_id=?`, r.db.Dialect),
		  strings.TrimSpace(proofID),
	  )
	  return scanProof(row)
   }

  func (r *SQLDriftProofRepo) ListProofsByLease(ctx context.Context, leaseID string) ([]*TreasDriftProof, error) {
	  rows,
		   err := r.db.Conn(ctx).QueryContext(ctx,
		   persist.Rewrite(`SELECT proof_id,lease_id,exec_id,asset,
		   record_snapshot_json,recheck_snapshot_json,
		   drift_abs,drift_ratio,decision,reason,created_at
		FROM treasury_exec_drift_proofs WHERE lease_id=? ORDER BY created_at ASC`, r.db.Dialect),
		  strings.TrimSpace(leaseID),
	  )
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanProofs(rows)
   }

   func (r *SQLDriftProofRepo) ListProofsByExec(ctx context.Context, execID string) ([]*TreasDriftProof, error) {
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  persist.Rewrite(`SELECT proof_id,lease_id,exec_id,asset,
		   record_snapshot_json,recheck_snapshot_json,
		   drift_abs,drift_ratio,decision,reason,created_at
		FROM treasury_exec_drift_proofs WHERE exec_id=? ORDER BY created_at ASC`, r.db.Dialect),
		  strings.TrimSpace(execID),
	   )
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanProofs(rows)
   }

// ──────────────────────────────────────────────────────────────────────────────
// SQLGenRepo
// ──────────────────────────────────────────────────────────────────────────────
// SQLGenRepo 基于 persist.SQLDB 的世代标记存储实现。
type SQLGenRepo struct {
	db *persist.SQLDB
}

// NewSQLGenRepo 构造 SQL 世代仓储。

   func NewSQLGenRepo(db *persist.SQLDB) *SQLGenRepo {
	   return &SQLGenRepo{db: db}
   }

  func (r *SQLGenRepo) SaveGen(ctx context.Context, g *ExecGeneration) error {
	  if g == nil || g.IdemKey == "" {
		   return ErrInvalidInput
	  }
	  if r.db.Dialect == persist.Postgres {
		   _, err := r.db.Conn(ctx).ExecContext(ctx, `
INSERT INTO treasury_exec_generations (gen_id,idem_key,generation,active_lease_id,updated_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (idem_key) DO UPDATE SET
  generation=EXCLUDED.generation,
  active_lease_id=EXCLUDED.active_lease_id,
  updated_at=EXCLUDED.updated_at`,
			   g.GenID, g.IdemKey, g.Generation, g.ActiveLeaseID, g.UpdatedAt.UTC(),
		   )
		  return err
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, `
INSERT INTO treasury_exec_generations (gen_id,idem_key,generation,active_lease_id,updated_at)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  generation=VALUES(generation),
  active_lease_id=VALUES(active_lease_id),
  updated_at=VALUES(updated_at)`,
		  g.GenID, g.IdemKey, g.Generation, g.ActiveLeaseID, g.UpdatedAt.UTC(),
	   )
	  return err
   }

   func (r *SQLGenRepo) GetGen(ctx context.Context, idemKey string) (*ExecGeneration, error) {
	   var g ExecGeneration
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  persist.Rewrite(`SELECT gen_id,idem_key,generation,active_lease_id,updated_at
		FROM treasury_exec_generations WHERE idem_key=?`, r.db.Dialect),
		   strings.TrimSpace(idemKey),
	  ).Scan(&g.GenID, &g.IdemKey, &g.Generation, &g.ActiveLeaseID, &g.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrLeaseNotFound
	  }
	  return &g, err
   }

// ──────────────────────────────────────────────────────────────────────────────
// 扫描辅助函数
// ──────────────────────────────────────────────────────────────────────────────

   func scanLease(row *sql.Row) (*PlanExecLease, error) {
	   var (
		  l        PlanExecLease
		   snapJSON string
		   stateStr string
		   ttlSeconds int64
		  recheckAt  sql.NullTime
		  sentAt sql.NullTime
		   ackedAt   sql.NullTime
		   blockedAt sql.NullTime
	  )
	   err := row.Scan(
		   &l.LeaseID, &l.IdemKey, &l.ExecID, &l.Asset, &l.Amount, &snapJSON,
		   &stateStr, &ttlSeconds, &l.BoundAt, &l.ExpiresAt,
		  &recheckAt, &sentAt, &ackedAt, &blockedAt, &l.BlockReason,
	   )
	   if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrLeaseNotFound
	   }
	  if err != nil {
		  return nil, err
	   }
	   l.State = LeaseState(stateStr)
	  l.TTL = time.Duration(ttlSeconds) * time.Second
	   if err := json.Unmarshal([]byte(snapJSON), &l.SnapshotDigest); err != nil {
		   return nil, err
	  }
	  if recheckAt.Valid {
		   t := recheckAt.Time
		   l.RecheckAt = &t
	   }
	   if sentAt.Valid {
		   t := sentAt.Time
		   l.SentAt = &t
	  }
	  if ackedAt.Valid {
		  t := ackedAt.Time
		   l.AckedAt = &t
	  }
	  if blockedAt.Valid {
		  t := blockedAt.Time
		  l.BlockedAt = &t
	  }
	  return &l, nil
   }

  func scanLeases(rows *sql.Rows) ([]*PlanExecLease, error) {
	  var out []*PlanExecLease
	   for rows.Next() {
		   var (
			   l PlanExecLease
			  snapJSON string
			   stateStr string
			   ttlSeconds int64
			   recheckAt  sql.NullTime
			   sentAt     sql.NullTime
			   ackedAt    sql.NullTime
			   blockedAt  sql.NullTime
		   )
		  if err := rows.Scan(
			  &l.LeaseID, &l.IdemKey, &l.ExecID, &l.Asset, &l.Amount, &snapJSON,
			  &stateStr, &ttlSeconds, &l.BoundAt, &l.ExpiresAt,
			  &recheckAt, &sentAt, &ackedAt, &blockedAt, &l.BlockReason,
		   ); err != nil {
			   return out, err
		   }
		  l.State = LeaseState(stateStr)
		  l.TTL = time.Duration(ttlSeconds) * time.Second
		  if err := json.Unmarshal([]byte(snapJSON), &l.SnapshotDigest); err != nil {
			  return out, err
		  }
		   if recheckAt.Valid {
			   t := recheckAt.Time
			   l.RecheckAt = &t
		   }
		  if sentAt.Valid {
			  t := sentAt.Time
			  l.SentAt = &t
		  }
		  if ackedAt.Valid {
			   t := ackedAt.Time
			  l.AckedAt = &t
		  }
		  if blockedAt.Valid {
			   t := blockedAt.Time
			  l.BlockedAt = &t
		  }
		  lcp := l
		   out = append(out, &lcp)
	  }
	   return out, rows.Err()
  }

   func scanProof(row *sql.Row) (*TreasDriftProof, error) {
	  var (
		  p           TreasDriftProof
		   recJSON     string
		  recheckJSON string
	  )
	  err := row.Scan(
		   &p.ProofID, &p.LeaseID, &p.ExecID, &p.Asset,
		   &recJSON, &recheckJSON,
		  &p.DriftAbs, &p.DriftRatio, &p.Decision, &p.Reason, &p.CreatedAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrLeaseNotFound
	  }
	   if err != nil {
		   return nil, err
	   }
	  _ = json.Unmarshal([]byte(recJSON), &p.RecordDigest)
	   _ = json.Unmarshal([]byte(recheckJSON), &p.RecheckDigest)
	  return &p, nil
  }

   func scanProofs(rows *sql.Rows) ([]*TreasDriftProof, error) {
	  var out []*TreasDriftProof
	   for rows.Next() {
		   var (
			   p       TreasDriftProof
			  recJSON string
			   recheckJSON string
		   )
		   if err := rows.Scan(
			   &p.ProofID, &p.LeaseID, &p.ExecID, &p.Asset,
			   &recJSON, &recheckJSON,
			   &p.DriftAbs, &p.DriftRatio, &p.Decision, &p.Reason, &p.CreatedAt,
		   ); err != nil {
			   return out, err
		   }
		  _ = json.Unmarshal([]byte(recJSON), &p.RecordDigest)
		   _ = json.Unmarshal([]byte(recheckJSON), &p.RecheckDigest)
		  pcp := p
		   out = append(out, &pcp)
	  }
	   return out, rows.Err()
  }

// nullableTime 将 *time.Time 转为 SQL 空值友好类型。
  func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC()
}
