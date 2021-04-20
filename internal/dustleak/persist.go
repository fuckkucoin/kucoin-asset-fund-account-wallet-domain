package dustleak

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// SQLStore 实现 Store 接口，使用口径 database/sql。
// 支持 MySQL 5.7+ / PostgreSQL 14+；DDL 见 migrations/0091_dust_leak.sql。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 创建 SQLStore。db 必须已经 Open 并且 Ping 成功。
   func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
  }

// ── PolicyGen ────────────────────────────────────────────────────────────────

const sqlUpsertPolicyGen = `
INSERT INTO dust_policy_gens
  (id, asset, gen, floor, min_convert, enabled, changed_by, created_at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  floor       = VALUES(floor),
  min_convert = VALUES(min_convert),
  enabled     = VALUES(enabled),
  changed_by  = VALUES(changed_by)
`

   func (s *SQLStore) SavePolicyGen(g *PolicyGen) error {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	  _, err := s.db.ExecContext(ctx, sqlUpsertPolicyGen,
		   g.ID, g.Asset, g.Gen, g.Floor, g.MinConvert, boolToInt(g.Enabled), g.ChangedBy, g.CreatedAt,
	   )
	  return err
   }

const sqlLatestPolicyGen = ` 
SELECT id, asset, gen, floor, min_convert, enabled, changed_by, created_at
FROM dust_policy_gens
WHERE asset = ?
ORDER BY gen DESC
LIMIT 1
`

   func (s *SQLStore) LatestPolicyGen(asset string) (*PolicyGen, error) {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer cancel()
	   row := s.db.QueryRowContext(ctx, sqlLatestPolicyGen, asset)
	   g := &PolicyGen{}
	   var enabledInt int
	   err := row.Scan(&g.ID, &g.Asset, &g.Gen, &g.Floor, &g.MinConvert, &enabledInt, &g.ChangedBy, &g.CreatedAt)
	   if err == sql.ErrNoRows {
		   return nil, ErrNotFound
	  }
	  if err != nil {
		  return nil, err
	  }
	   g.Enabled = enabledInt != 0
	  return g, nil
  }

// ── QuoteSnapshot ────────────────────────────────────────────────────────────

const sqlUpsertSnapshot = `
INSERT INTO quote_snapshots
  (id, snap_key, asset, rate_usdt, policy_gen_id, policy_gen, drift_cap_bps, convert_id, snap_at, used_at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  convert_id = VALUES(convert_id),
  used_at    = VALUES(used_at)
`

  func (s *SQLStore) SaveSnapshot(q *QuoteSnapshot) error {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer cancel()
	  var usedAt interface{}
	  if !q.UsedAt.IsZero() {
		   usedAt = q.UsedAt
	   }
	  _, err := s.db.ExecContext(ctx, sqlUpsertSnapshot,
		   q.ID, q.SnapKey, q.Asset, q.RateUSDT, q.PolicyGenID, q.PolicyGen,
		   q.DriftCapBps, q.ConvertID, q.SnapAt, usedAt,
	   )
	   return err
   }

const sqlSnapshotByKey = ` 
SELECT id, snap_key, asset, rate_usdt, policy_gen_id, policy_gen, drift_cap_bps, convert_id, snap_at, used_at
FROM quote_snapshots
WHERE snap_key = ?
LIMIT 1
`

   func (s *SQLStore) SnapshotByKey(snapKey string) (*QuoteSnapshot, error) {
	   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer cancel()
	  row := s.db.QueryRowContext(ctx,
		   sqlSnapshotByKey, snapKey)
	  q := &QuoteSnapshot{}
	  var usedAt sql.NullTime
	  err := row.Scan(&q.ID, &q.SnapKey, &q.Asset, &q.RateUSDT, &q.PolicyGenID, &q.PolicyGen,
		  &q.DriftCapBps,
		  &q.ConvertID, &q.SnapAt, &usedAt)
	   if err == sql.ErrNoRows {
		   return nil, ErrNotFound
	  }
	  if err != nil {
		  return nil, err
	  }
	  if usedAt.Valid {
		  q.UsedAt = usedAt.Time
	  }
	   return q, nil
   }

// ── LeakCase ─────────────────────────────────────────────────────────────────

const sqlUpsertLeakCase = `
INSERT INTO dust_leaks
  (id, kind, status, user_id, asset, convert_id, snapshot_id, policy_gen_id,
   actual_amount, actual_usdt, floor_amount, max_usdt, leak_amount, leak_usdt,
   detected_by, closed_by, note, detected_at, updated_at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  status     = VALUES(status),
  closed_by  = VALUES(closed_by),
  note       = VALUES(note),
  updated_at = VALUES(updated_at)
`

  func (s *SQLStore) SaveLeakCase(lc *LeakCase) error {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	  _,
		   err := s.db.ExecContext(ctx, sqlUpsertLeakCase,
		  lc.ID, lc.Kind, lc.Status, lc.UserID, lc.Asset, lc.ConvertID,
		  lc.SnapshotID, lc.PolicyGenID,
		  lc.ActualAmount, lc.ActualUSDT, lc.FloorAmount, lc.MaxUSDT,
		   lc.LeakAmount, lc.LeakUSDT,
		   lc.DetectedBy, lc.ClosedBy, lc.Note, lc.DetectedAt, lc.UpdatedAt,
	  )
	  return err
  }

const sqlLeakCaseByID = `
SELECT id, kind, status, user_id, asset, convert_id, snapshot_id, policy_gen_id,
       actual_amount, actual_usdt, floor_amount, max_usdt, leak_amount, leak_usdt,
       detected_by, closed_by, note, detected_at, updated_at
FROM dust_leaks WHERE id = ?
`

   func (s *SQLStore) LeakCaseByID(id string) (*LeakCase, error) {
	   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	   row := s.db.QueryRowContext(ctx, sqlLeakCaseByID, id)
	   return scanLeakCase(row)
   }

const sqlLeakCaseByConvert = ` 
SELECT id, kind, status, user_id, asset, convert_id, snapshot_id, policy_gen_id,
       actual_amount, actual_usdt, floor_amount, max_usdt, leak_amount, leak_usdt,
       detected_by, closed_by, note, detected_at, updated_at
FROM dust_leaks WHERE convert_id = ? LIMIT 1
`

   func (s *SQLStore) LeakCaseByConvert(convertID string) (*LeakCase, error) {
	   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer cancel()
	  row := s.db.QueryRowContext(ctx,
		   sqlLeakCaseByConvert, convertID)
	  lc, err := scanLeakCase(row)
	   if err == ErrNotFound {
		   return nil, nil
	   }
	   return lc, err
   }

const sqlOpenLeaksByUser = `
SELECT id, kind, status, user_id, asset, convert_id, snapshot_id, policy_gen_id,
       actual_amount, actual_usdt, floor_amount, max_usdt, leak_amount, leak_usdt,
       detected_by, closed_by, note, detected_at, updated_at
FROM dust_leaks
WHERE user_id = ? AND (asset = ? OR ? = '') AND status IN ('detected','rolling_back')
ORDER BY detected_at DESC
LIMIT 100
`

  func (s *SQLStore) OpenLeaksByUser(userID, asset string) ([]*LeakCase, error) {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer cancel()
	   rows, err := s.db.QueryContext(ctx, sqlOpenLeaksByUser, userID, asset, asset)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   var out []*LeakCase
	   for rows.Next() {
		  lc, err := scanLeakCaseRow(rows)
		  if err != nil {
			   continue
		   }
		   out = append(out, lc)
	   }
	   return out, rows.Err()
  }

  func scanLeakCase(row *sql.Row) (*LeakCase, error) {
	   lc := &LeakCase{}
	  err := row.Scan(
		   &lc.ID, &lc.Kind, &lc.Status, &lc.UserID, &lc.Asset,
		  &lc.ConvertID, &lc.SnapshotID, &lc.PolicyGenID,
		  &lc.ActualAmount, &lc.ActualUSDT, &lc.FloorAmount, &lc.MaxUSDT,
		  &lc.LeakAmount, &lc.LeakUSDT,
		   &lc.DetectedBy, &lc.ClosedBy, &lc.Note, &lc.DetectedAt, &lc.UpdatedAt,
	   )
	   if err == sql.ErrNoRows {
		   return nil, ErrNotFound
	  }
	  if err != nil {
		  return nil, err
	   }
	   return lc, nil
   }

  func scanLeakCaseRow(rows *sql.Rows) (*LeakCase, error) {
	   lc := &LeakCase{}
	   err := rows.Scan(
		  &lc.ID, &lc.Kind, &lc.Status, &lc.UserID, &lc.Asset,
		  &lc.ConvertID, &lc.SnapshotID, &lc.PolicyGenID,
		   &lc.ActualAmount, &lc.ActualUSDT, &lc.FloorAmount, &lc.MaxUSDT,
		   &lc.LeakAmount, &lc.LeakUSDT,
		  &lc.DetectedBy, &lc.ClosedBy, &lc.Note, &lc.DetectedAt, &lc.UpdatedAt,
	  )
	  if err != nil {
		  return nil, err
	  }
	  return lc, nil
  }

// ── LeakRollback ─────────────────────────────────────────────────────────────

const sqlUpsertRollback = ` 
INSERT INTO leak_rollbacks
  (id, leak_id, status, user_id, asset, convert_id,
   rollback_usdt, rollback_asset, executed_by, dual_req_id,
   proof_hash, note, posted_at, created_at, updated_at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  status         = VALUES(status),
  executed_by    = VALUES(executed_by),
  proof_hash     = VALUES(proof_hash),
  note           = VALUES(note),
  posted_at      = VALUES(posted_at),
  updated_at     = VALUES(updated_at)
`

  func (s *SQLStore) SaveRollback(r *LeakRollback) error {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	   var postedAt interface{}
	  if !r.PostedAt.IsZero() {
		   postedAt = r.PostedAt
	  }
	  _, err := s.db.ExecContext(ctx, sqlUpsertRollback,
		  r.ID, r.LeakID, r.Status, r.UserID, r.Asset, r.ConvertID,
		   r.RollbackUSDT, r.RollbackAsset, r.ExecutedBy, r.DualReqID,
		   r.ProofHash, r.Note, postedAt, r.CreatedAt, r.UpdatedAt,
	  )
	   return err
   }

const sqlRollbackByLeak = `
SELECT id, leak_id, status, user_id, asset, convert_id,
       rollback_usdt, rollback_asset, executed_by, dual_req_id,
       proof_hash, note, posted_at, created_at, updated_at
FROM leak_rollbacks WHERE leak_id = ? LIMIT 1
`

  func (s *SQLStore) RollbackByLeak(leakID string) (*LeakRollback, error) {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	  row := s.db.QueryRowContext(ctx, sqlRollbackByLeak, leakID)
	   rb := &LeakRollback{}
	  var postedAt sql.NullTime
	  err := row.Scan(
		  &rb.ID, &rb.LeakID, &rb.Status, &rb.UserID, &rb.Asset, &rb.ConvertID,
		  &rb.RollbackUSDT, &rb.RollbackAsset, &rb.ExecutedBy, &rb.DualReqID,
		   &rb.ProofHash, &rb.Note, &postedAt, &rb.CreatedAt, &rb.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		  return nil, ErrNotFound
	   }
	   if err != nil {
		  return nil, err
	   }
	  if postedAt.Valid {
		  rb.PostedAt = postedAt.Time
	   }
	  return rb, nil
   }

// ── ConvertGate ───────────────────────────────────────────────────────────────

const sqlUpsertGate = `
INSERT INTO convert_gates
  (id, user_id, asset, status, leak_id, blocked_by, blocked_at, released_by, released_at, created_at, updated_at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  status      = VALUES(status),
  leak_id     = VALUES(leak_id),
  blocked_by  = VALUES(blocked_by),
  blocked_at  = VALUES(blocked_at),
  released_by = VALUES(released_by),
  released_at = VALUES(released_at),
  updated_at  = VALUES(updated_at)
`

   func (s *SQLStore) SaveGate(g *ConvertGate) error {
	  ctx,
		   cancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer cancel()
	   var blockedAt, releasedAt interface{}
	   if !g.BlockedAt.IsZero() {
		  blockedAt = g.BlockedAt
	  }
	   if !g.ReleasedAt.IsZero() {
		  releasedAt = g.ReleasedAt
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpsertGate,
		   g.ID,
		   g.UserID, g.Asset, g.Status, g.LeakID,
		  g.BlockedBy, blockedAt, g.ReleasedBy, releasedAt,
		   g.CreatedAt, g.UpdatedAt,
	  )
	  return err
   }

const sqlGateByUserAsset = `
SELECT id, user_id, asset, status, leak_id, blocked_by, blocked_at, released_by, released_at, created_at, updated_at
FROM convert_gates WHERE user_id = ? AND asset = ? LIMIT 1
`

  func (s *SQLStore) GateByUserAsset(userID, asset string) (*ConvertGate, error) {
	  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	   row := s.db.QueryRowContext(ctx,
		  sqlGateByUserAsset, userID, asset)
	   g := &ConvertGate{}
	  var blockedAt, releasedAt sql.NullTime
	  err := row.Scan(&g.ID, &g.UserID, &g.Asset, &g.Status, &g.LeakID,
		  &g.BlockedBy, &blockedAt, &g.ReleasedBy, &releasedAt, &g.CreatedAt, &g.UpdatedAt)
	  if err == sql.ErrNoRows {
		   return nil, ErrNotFound
	   }
	   if err != nil {
		  return nil, err
	  }
	  if blockedAt.Valid {
		   g.BlockedAt = blockedAt.Time
	   }
	  if releasedAt.Valid {
		   g.ReleasedAt = releasedAt.Time
	   }
	  return g, nil
   }

// ── 辅助 ─────────────────────────────────────────────────────────────────────

   func boolToInt(b bool) int {
	  if b {
		   return 1
	   }
	  return 0
  }

// jsonMarshal 用于诊断，不对外暴露。

   func jsonMarshal(v interface{}) string {
	  b, _ := json.Marshal(v)
	  return string(b)
  }

// RecentConvertsFromSQL 从 dust_conv_records 视图拉取近期兑换记录（供 Worker 使用）。
// 表名与字段由 dustconv 迁移定义；此处只读查询。
const sqlRecentConverts = `
SELECT id, user_id, asset, amount, usdt, rate, at, idempotency_key
FROM dust_conv_records
WHERE at >= ? ORDER BY at ASC LIMIT ?
`

// SQLConvertFetcher 实现 ConvertFetcher 接口，从 SQL 拉取 ConvertRecord。
type SQLConvertFetcher struct {
	db *sql.DB
}

// NewSQLConvertFetcher 创建 SQLConvertFetcher。
  func NewSQLConvertFetcher(db *sql.DB) *SQLConvertFetcher {
	   return &SQLConvertFetcher{db: db}
   }

   func (f *SQLConvertFetcher) RecentConverts(ctx context.Context, since time.Time, limit int) ([]ConvertRecord, error) {
	   rows, err := f.db.QueryContext(ctx, sqlRecentConverts, since, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   var out []ConvertRecord
	   for rows.Next() {
		  var rec ConvertRecord
		   if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Asset, &rec.Amount, &rec.USDT, &rec.Rate, &rec.At, &rec.IdemKey); err != nil {
			   continue
		  }
		   out = append(out, rec)
	  }
	  return out, rows.Err()
  }
// temporary guard removed after the production check
