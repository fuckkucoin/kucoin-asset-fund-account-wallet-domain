// 功能：sqlstore.go
// batchhopgate SQLStore——MySQL/PostgreSQL 持久化实现。
// SQLStore 实现 Store 接口，对接 portfolio_batch_residuals、portfolio_resume_leases、
// portfolio_hop_proofs、portfolio_batch_generations 四张表。
// 设计原则：
// - CAS 协议（BumpGeneration）在 SQL 层通过原子 UPDATE ... WHERE generation=? AND status='idle'
// 实现；影响行数=0 → ErrStaleGeneration 或 ErrConcurrentBatch；=1 → 成功。
// - FindOpenResidual 走 (user_id, asset, status) 复合索引；status IN ('open','proven','resuming')。
// - FindActiveLease 走 (residual_case_id, status) 复合索引；status IN ('open','bound')。
// - 所有金额列 DECIMAL(36,18)；时间列 DATETIME(3)（精度 1ms）。
// - 不依赖 ORM；所有 SQL 手写，便于审计与。
// 主机占位（保持产品契约不变，不修改）：
// pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
// pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
// CreateDate: 2022.02.24

// 作者：Cason
// 日期：2021.02.24

package batchhopgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
	"github.com/google/uuid"
)

// SQLStore 是 Store 的 SQL 数据库实现。
type SQLStore struct {
	db *sql.DB
	clock func() time.Time
}

// NewSQLStore 构造 SQL 存储实例。db 须已初始化并已执行 migrations/0126_aa_batchhopgate.sql。

   func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db, clock: func() time.Time { return time.Now().UTC() }}
   }

   func (s *SQLStore) WithClock(c func() time.Time) *SQLStore {
	   if s != nil && c != nil {
		   s.clock = c
	   }
	  return s
  }

   // ─────────────────────────────────────────────────────────────────────────────
  // BatchResidualCase CRUD
   // ─────────────────────────────────────────────────────────────────────────────
   // SaveResidual 以 upsert 方式写入残留案（按 ID）。

   func (s *SQLStore) SaveResidual(ctx context.Context, c *BatchResidualCase) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	   hopsJSON, err := json.Marshal(c.SucceededHops)
	   if err != nil {
		  return err
	  }
	   q := `
INSERT INTO portfolio_batch_residuals
  (id, user_id, asset, generation, total_items, fail_index,
   succeeded_hops, failed_reason, before_root, partial_root,
   status, proof_id, lease_id, confirm1_by, confirm2_by,
   confirm1_at, confirm2_at, note, detected_at, updated_at)
VALUES
  (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  status=VALUES(status),
  proof_id=VALUES(proof_id),
  lease_id=VALUES(lease_id),
  confirm1_by=VALUES(confirm1_by),
  confirm2_by=VALUES(confirm2_by),
  confirm1_at=VALUES(confirm1_at),
  confirm2_at=VALUES(confirm2_at),
  note=VALUES(note),
  updated_at=VALUES(updated_at)
`
	  _, err = s.db.ExecContext(ctx, q,
		  c.ID,
		  c.UserID, c.Asset, c.Generation, c.TotalItems, c.FailIndex,
		   string(hopsJSON), c.FailedReason, c.BeforeRoot, c.PartialRoot,
		  string(c.Status), c.ProofID, c.LeaseID, c.Confirm1By, c.Confirm2By,
		   nullTime(c.Confirm1At), nullTime(c.Confirm2At), c.Note,
		   c.DetectedAt.UTC(), c.UpdatedAt.UTC(),
	  )
	   return err
   }

// GetResidual 按 case_id 查询。
   func (s *SQLStore) GetResidual(ctx context.Context, caseID string) (*BatchResidualCase, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   q := `SELECT id, user_id, asset, generation, total_items, fail_index,
         succeeded_hops, failed_reason, before_root, partial_root,
         status, proof_id, lease_id, confirm1_by, confirm2_by,
         confirm1_at, confirm2_at, note, detected_at, updated_at
         FROM portfolio_batch_residuals WHERE id = ?`
	   row := s.db.QueryRowContext(ctx, q, caseID)
	   return scanResidual(row)
  }

// FindOpenResidual 按 user_id + asset 查找非终态残留案。

   func (s *SQLStore) FindOpenResidual(ctx context.Context, userID, asset string) (*BatchResidualCase, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   q := `SELECT id, user_id, asset, generation, total_items, fail_index,
         succeeded_hops, failed_reason, before_root, partial_root,
         status, proof_id, lease_id, confirm1_by, confirm2_by,
         confirm1_at, confirm2_at, note, detected_at, updated_at
         FROM portfolio_batch_residuals
         WHERE user_id = ? AND asset = ? AND status IN ('open','proven','resuming')
         LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, userID, strings.ToUpper(strings.TrimSpace(asset)))
	   c, err := scanResidual(row)
	   if err == sql.ErrNoRows || err == ErrResidualNotFound {
		   return nil, nil
	  }
	   return c, err
  }

// ListResidualsByUser 返回用户所有残留案，按 detected_at 降序。
   func (s *SQLStore) ListResidualsByUser(ctx context.Context, userID string, limit int) ([]*BatchResidualCase, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  q := `SELECT id, user_id, asset, generation, total_items, fail_index,
         succeeded_hops, failed_reason, before_root, partial_root,
         status, proof_id, lease_id, confirm1_by, confirm2_by,
         confirm1_at, confirm2_at, note, detected_at, updated_at
         FROM portfolio_batch_residuals
         WHERE user_id = ?
         ORDER BY detected_at DESC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, userID, limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanResiduals(rows)
   }

// ListResidualsByStatus 按状态批量查询，按 detected_at 升序。
   func (s *SQLStore) ListResidualsByStatus(ctx context.Context, status ResidualStatus, limit int) ([]*BatchResidualCase, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   q := `SELECT id, user_id, asset, generation, total_items, fail_index,
         succeeded_hops, failed_reason, before_root, partial_root,
         status, proof_id, lease_id, confirm1_by, confirm2_by,
         confirm1_at, confirm2_at, note, detected_at, updated_at
         FROM portfolio_batch_residuals
         WHERE status = ?
         ORDER BY detected_at ASC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, string(status), limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanResiduals(rows)
   }

// ─────────────────────────────────────────────────────────────────────────────
// ResumeLease CRUD
// ─────────────────────────────────────────────────────────────────────────────
// SaveLease 以 upsert 方式写入续跳租约。
  func (s *SQLStore) SaveLease(ctx context.Context, l *ResumeLease) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	   q := `
INSERT INTO portfolio_resume_leases
  (id, residual_case_id, user_id, asset, generation, resume_from,
   bound_idem, status, expires_at, confirmed_by, confirmed_at, note, created_at, updated_at)
VALUES
  (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  bound_idem=VALUES(bound_idem),
  status=VALUES(status),
  confirmed_by=VALUES(confirmed_by),
  confirmed_at=VALUES(confirmed_at),
  note=VALUES(note),
  updated_at=VALUES(updated_at)
`
	  _, err := s.db.ExecContext(ctx, q,
		   l.ID, l.ResidualCaseID, l.UserID, l.Asset, l.Generation, l.ResumeFrom,
		  l.BoundIdem, string(l.Status), l.ExpiresAt.UTC(),
		  l.ConfirmedBy, nullTime(l.ConfirmedAt), l.Note,
		   l.CreatedAt.UTC(),
		  l.UpdatedAt.UTC(),
	   )
	   return err
   }

// GetLease 按 lease_id 查询。

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*ResumeLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   q := `SELECT id, residual_case_id, user_id, asset, generation, resume_from, 
         bound_idem, status, expires_at, confirmed_by, confirmed_at, note, created_at, updated_at
         FROM portfolio_resume_leases WHERE id = ?`
	   row := s.db.QueryRowContext(ctx, q, leaseID)
	  return scanLease(row)
  }

// FindActiveLease 按 residual_case_id 查找活跃租约（open/bound）。
  func (s *SQLStore) FindActiveLease(ctx context.Context, residualCaseID string) (*ResumeLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  q := `SELECT id, residual_case_id, user_id, asset, generation, resume_from,
         bound_idem, status, expires_at, confirmed_by, confirmed_at, note, created_at, updated_at
         FROM portfolio_resume_leases
         WHERE residual_case_id = ? AND status IN ('open','bound')
         ORDER BY created_at DESC LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, residualCaseID)
	   l, err := scanLease(row)
	  if err == sql.ErrNoRows || err == ErrLeaseNotFound {
		   return nil, nil
	   }
	   return l, err
  }

// ListExpiredOpenLeases 返回超过 ExpiresAt 且状态为 open 的租约。

   func (s *SQLStore) ListExpiredOpenLeases(ctx context.Context, before time.Time, limit int) ([]*ResumeLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   q := `SELECT id, residual_case_id, user_id, asset, generation, resume_from,
         bound_idem, status, expires_at, confirmed_by, confirmed_at, note, created_at, updated_at
         FROM portfolio_resume_leases
         WHERE status = 'open' AND expires_at < ?
         ORDER BY expires_at ASC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, before.UTC(), limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  return scanLeases(rows)
  }

// ─────────────────────────────────────────────────────────────────────────────
// HopOccupyProof CRUD
// ─────────────────────────────────────────────────────────────────────────────
// SaveProof 以 upsert 方式写入占用证明。

   func (s *SQLStore) SaveProof(ctx context.Context, p *HopOccupyProof) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	  totalsJSON, err := json.Marshal(p.SubjectTotals)
	   if err != nil {
		   return err
	   }
	   q := `
INSERT INTO portfolio_hop_proofs
  (id, residual_case_id, user_id, asset, before_root, partial_root,
   subject_totals, conserve_ok, digest, built_by, verified_at, created_at)
VALUES
  (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  conserve_ok=VALUES(conserve_ok),
  digest=VALUES(digest),
  built_by=VALUES(built_by),
  verified_at=VALUES(verified_at)
`
	  _, err = s.db.ExecContext(ctx, q,
		  p.ID, p.ResidualCaseID, p.UserID, p.Asset,
		  p.BeforeRoot, p.PartialRoot,
		  string(totalsJSON),
		  p.ConserveOK, p.Digest,
		   p.BuiltBy, p.VerifiedAt.UTC(), p.CreatedAt.UTC(),
	  )
	  return err
  }

// GetProof 按 proof_id 查询。

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*HopOccupyProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  q := `SELECT id, residual_case_id, user_id, asset, before_root, partial_root,
         subject_totals, conserve_ok, digest, built_by, verified_at, created_at
         FROM portfolio_hop_proofs WHERE id = ?`
	   row := s.db.QueryRowContext(ctx, q, proofID)
	  return scanProof(row)
  }

// GetProofByCase 按 residual_case_id 查找关联证明。

   func (s *SQLStore) GetProofByCase(ctx context.Context, residualCaseID string) (*HopOccupyProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   q := `SELECT id, residual_case_id, user_id, asset, before_root, partial_root,
         subject_totals, conserve_ok, digest, built_by, verified_at, created_at
         FROM portfolio_hop_proofs WHERE residual_case_id = ? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, residualCaseID)
	  p, err := scanProof(row)
	  if err == sql.ErrNoRows || err == ErrProofNotFound {
		  return nil, nil
	   }
	   return p, err
  }

// ─────────────────────────────────────────────────────────────────────────────
// BatchGeneration CAS
// ─────────────────────────────────────────────────────────────────────────────
// GetGeneration 返回用户资产 BatchGeneration 记录，不存在时自动初始化。
  func (s *SQLStore) GetGeneration(ctx context.Context, userID, asset string) (*BatchGeneration, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   q := `SELECT id, user_id, asset, generation, status, updated_at
         FROM portfolio_batch_generations WHERE user_id = ? AND asset = ?`
	   row := s.db.QueryRowContext(ctx, q, userID, asset)
	  var g BatchGeneration
	   var updatedAt time.Time
	   err := row.Scan(&g.ID, &g.UserID, &g.Asset, &g.Generation, &g.Status, &updatedAt)
	  if err == sql.ErrNoRows {
		  g = BatchGeneration{
			   ID:     uuid.New().String(),
			   UserID: userID,
			   Asset:      asset,
			   Generation: 0,
			  Status:     "idle",
			   UpdatedAt: time.Now().UTC(),
		  }
		  iq := `INSERT INTO portfolio_batch_generations (id, user_id, asset, generation, status, updated_at)
               VALUES (?,?,?,?,?,?) ON DUPLICATE KEY UPDATE id=id`
		  _,
			  _ = s.db.ExecContext(ctx, iq, g.ID, g.UserID, g.Asset, g.Generation, g.Status, g.UpdatedAt.UTC())
		   return &g, nil
	  }
	   if err != nil {
		  return nil, err
	  }
	   g.UpdatedAt = updatedAt
	  return &g, nil
  }

// BumpGeneration CAS 递增世代，SQL 层原子执行。
   func (s *SQLStore) BumpGeneration(ctx context.Context, userID, asset string, expected int64) (int64, error) {
	  if s == nil || s.db == nil {
		   return 0, ErrUnavailable
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   // 先检查是否已为 active（并行 BatchHop 互斥）。
	   chk := `SELECT status, generation FROM portfolio_batch_generations WHERE user_id=? AND asset=?`
	  row := s.db.QueryRowContext(ctx,
		   chk, userID, asset)
	  var status string
	  var curGen int64
	  if err := row.Scan(&status, &curGen); err != nil {
		   if err == sql.ErrNoRows {
			   if expected != 0 {
				   return 0, ErrStaleGeneration
			  }
			   id := uuid.New().String()
			   ins := `INSERT INTO portfolio_batch_generations (id, user_id, asset, generation, status, updated_at)
                    VALUES (?,?,?,1,'active',NOW(3)) ON DUPLICATE KEY UPDATE id=id`
			  res, err := s.db.ExecContext(ctx, ins, id, userID, asset)
			  if err != nil {
				  return 0, err
			  }
			  if n, _ := res.RowsAffected(); n == 0 {
				   return 0, ErrStaleGeneration
			   }
			  return 1, nil
		  }
		   return 0, err
	   }
	  if status == "active" {
		  return 0, ErrConcurrentBatch
	  }
	  upd := `UPDATE portfolio_batch_generations
            SET generation = generation + 1, status = 'active', updated_at = NOW(3)
            WHERE user_id = ? AND asset = ? AND generation = ? AND status = 'idle'`
	   res, err := s.db.ExecContext(ctx, upd, userID, asset, expected)
	  if err != nil {
		   return 0, err
	   }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		  return 0, ErrStaleGeneration
	   }
	   return expected + 1, nil
  }

// ReleaseGeneration 将世代状态切换回 idle。

   func (s *SQLStore) ReleaseGeneration(ctx context.Context, userID, asset string, generation int64) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	  upd := `UPDATE portfolio_batch_generations
            SET status = 'idle', updated_at = NOW(3)
            WHERE user_id = ? AND asset = ? AND generation = ?`
	  _, err := s.db.ExecContext(ctx, upd, userID, strings.ToUpper(strings.TrimSpace(asset)), generation)
	   return err
   }

// ─────────────────────────────────────────────────────────────────────────────
// 扫描辅助函数
// ─────────────────────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

  func scanResidual(row rowScanner) (*BatchResidualCase, error) {
	   var c BatchResidualCase
	   var hopsJSON string
	  var status string
	   var c1at, c2at sql.NullTime
	   err := row.Scan(
		   &c.ID,
		  &c.UserID, &c.Asset, &c.Generation, &c.TotalItems, &c.FailIndex,
		   &hopsJSON, &c.FailedReason, &c.BeforeRoot, &c.PartialRoot,
		  &status, &c.ProofID, &c.LeaseID, &c.Confirm1By, &c.Confirm2By,
		   &c1at, &c2at, &c.Note, &c.DetectedAt, &c.UpdatedAt,
	  )
	  if err == sql.ErrNoRows {
		   return nil, ErrResidualNotFound
	   }
	  if err != nil {
		   return nil, err
	   }
	  c.Status = ResidualStatus(status)
	   _ = json.Unmarshal([]byte(hopsJSON), &c.SucceededHops)
	   if c1at.Valid {
		   t := c1at.Time
		  c.Confirm1At = &t
	   }
	  if c2at.Valid {
		   t := c2at.Time
		   c.Confirm2At = &t
	  }
	   return &c, nil
   }

   func scanResiduals(rows *sql.Rows) ([]*BatchResidualCase, error) {
	   var out []*BatchResidualCase
	  for rows.Next() {
		  var c BatchResidualCase
		  var hopsJSON string
		   var status string
		  var c1at, c2at sql.NullTime
		   err := rows.Scan(
			   &c.ID,
			   &c.UserID, &c.Asset, &c.Generation, &c.TotalItems, &c.FailIndex,
			   &hopsJSON,
			   &c.FailedReason, &c.BeforeRoot, &c.PartialRoot,
			   &status, &c.ProofID, &c.LeaseID, &c.Confirm1By, &c.Confirm2By,
			  &c1at, &c2at, &c.Note, &c.DetectedAt, &c.UpdatedAt,
		   )
		  if err != nil {
			  return nil, err
		   }
		   c.Status = ResidualStatus(status)
		  _ = json.Unmarshal([]byte(hopsJSON), &c.SucceededHops)
		   if c1at.Valid {
			   t := c1at.Time
			   c.Confirm1At = &t
		   }
		  if c2at.Valid {
			   t := c2at.Time
			   c.Confirm2At = &t
		   }
		   cp := c
		   out = append(out, &cp)
	  }
	  return out, rows.Err()
  }

  func scanLease(row rowScanner) (*ResumeLease, error) {
	  var l ResumeLease
	  var status string
	   var confirmedAt sql.NullTime
	   err := row.Scan(
		   &l.ID, &l.ResidualCaseID, &l.UserID, &l.Asset, &l.Generation, &l.ResumeFrom,
		  &l.BoundIdem, &status, &l.ExpiresAt, &l.ConfirmedBy, &confirmedAt, &l.Note,
		   &l.CreatedAt, &l.UpdatedAt,
	   )
	  if err == sql.ErrNoRows {
		  return nil, ErrLeaseNotFound
	   }
	  if err != nil {
		   return nil, err
	  }
	  l.Status = LeaseStatus(status)
	  if confirmedAt.Valid {
		   t := confirmedAt.Time
		   l.ConfirmedAt = &t
	  }
	   return &l, nil
  }

   func scanLeases(rows *sql.Rows) ([]*ResumeLease, error) {
	  var out []*ResumeLease
	  for rows.Next() {
		   var l ResumeLease
		   var status string
		   var confirmedAt sql.NullTime
		   err := rows.Scan(
			   &l.ID,
			  &l.ResidualCaseID, &l.UserID, &l.Asset, &l.Generation, &l.ResumeFrom,
			   &l.BoundIdem, &status, &l.ExpiresAt, &l.ConfirmedBy, &confirmedAt, &l.Note,
			   &l.CreatedAt, &l.UpdatedAt,
		   )
		  if err != nil {
			  return nil, err
		   }
		  l.Status = LeaseStatus(status)
		  if confirmedAt.Valid {
			   t := confirmedAt.Time
			  l.ConfirmedAt = &t
		   }
		  cp := l
		   out = append(out, &cp)
	  }
	  return out, rows.Err()
  }

  func scanProof(row rowScanner) (*HopOccupyProof, error) {
	   var p HopOccupyProof
	   var totalsJSON string
	   var conserveOK int
	   err := row.Scan(
		   &p.ID,
		  &p.ResidualCaseID, &p.UserID, &p.Asset,
		  &p.BeforeRoot, &p.PartialRoot, &totalsJSON, &conserveOK,
		   &p.Digest, &p.BuiltBy, &p.VerifiedAt, &p.CreatedAt,
	  )
	  if err == sql.ErrNoRows {
		  return nil, ErrProofNotFound
	   }
	  if err != nil {
		   return nil, err
	  }
	   p.ConserveOK = conserveOK != 0
	  _ = json.Unmarshal([]byte(totalsJSON), &p.SubjectTotals)
	   return &p, nil
   }

// nullTime 将 *time.Time 转为 sql.NullTime（nil → null）。

   func nullTime(t *time.Time) sql.NullTime {
	  if t == nil {
		  return sql.NullTime{}
	   }
	   return sql.NullTime{Time: t.UTC(), Valid: true}
  }
