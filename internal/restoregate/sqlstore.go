package restoregate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// SQLStore 基于 SQL 的持久化实现。
// 适配 MySQL / PostgreSQL；使用口径 database/sql 接口，由调用方注入 *sql.DB。
// 迁移脚本在 migrations/0159_aa_restore_gate.sql。

type SQLStore struct {
	db    *sql.DB
	clock func() time.Time
}

// NewSQLStore 构造 SQLStore。db 不得为 nil。
   func NewSQLStore(db *sql.DB) *SQLStore {
	  if db == nil {
		   panic("restoregate: SQLStore requires non-nil *sql.DB")
	  }
	  return &SQLStore{db: db, clock: func() time.Time { return time.Now().UTC() }}
   }

// WithClock 注入时钟（用）。
  func (s *SQLStore) WithClock(c func() time.Time) *SQLStore {
	  s.clock = c
	  return s
  }

   // ── RestorePlan ─────────────────────────────────────────────────────────────

   func (s *SQLStore) CreatePlan(ctx context.Context, p RestorePlan) error {
	   q := `INSERT INTO restore_plans
		(id, blob_id, snap_id, target_gen, live_watermark, balance_root_pre, balance_root_post,
		 state, creator, reason, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		  p.ID, p.BlobID, p.SnapID, p.TargetGen, p.LiveWatermark,
		   p.BalanceRootPre, p.BalanceRootPost,
		  string(p.State), p.Creator, p.Reason,
		  p.CreatedAt, p.UpdatedAt,
	   )
	   if err != nil {
		  if isDuplicateKey(err) {
			  return ErrPlanConflict
		   }
		  return fmt.Errorf("restoregate.SQLStore.CreatePlan: %w", err)
	  }
	  return nil
   }

   func (s *SQLStore) GetPlan(ctx context.Context, id string) (RestorePlan, error) {
	   q := `SELECT id, blob_id, snap_id, target_gen, live_watermark, balance_root_pre, balance_root_post,
		state, creator, reason, created_at, updated_at,
		frozen_at, import_start_at, verify_start_at, sealed_at, aborted_at, abort_reason
		FROM restore_plans WHERE id=?`
	   row := s.db.QueryRowContext(ctx,
		  q, id)
	  var p RestorePlan
	  var state string
	  if err := row.Scan(
		   &p.ID, &p.BlobID, &p.SnapID, &p.TargetGen, &p.LiveWatermark,
		   &p.BalanceRootPre,
		   &p.BalanceRootPost,
		  &state, &p.Creator, &p.Reason, &p.CreatedAt, &p.UpdatedAt,
		   &p.FrozenAt,
		   &p.ImportStartAt, &p.VerifyStartAt, &p.SealedAt, &p.AbortedAt, &p.AbortReason,
	   ); err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return RestorePlan{}, ErrPlanNotFound
		  }
		   return RestorePlan{}, fmt.Errorf("restoregate.SQLStore.GetPlan: %w", err)
	   }
	  p.State = PlanState(state)
	  return p, nil
  }

   func (s *SQLStore) UpdatePlan(ctx context.Context, p RestorePlan) error {
	   q := `UPDATE restore_plans SET 
		blob_id=?, snap_id=?, target_gen=?, live_watermark=?,
		balance_root_pre=?, balance_root_post=?,
		state=?, creator=?, reason=?, updated_at=?,
		frozen_at=?, import_start_at=?, verify_start_at=?, sealed_at=?, aborted_at=?, abort_reason=?
		WHERE id=?`
	  res, err := s.db.ExecContext(ctx, q,
		  p.BlobID,
		  p.SnapID, p.TargetGen, p.LiveWatermark,
		   p.BalanceRootPre, p.BalanceRootPost,
		   string(p.State), p.Creator, p.Reason, p.UpdatedAt,
		  p.FrozenAt, p.ImportStartAt, p.VerifyStartAt, p.SealedAt, p.AbortedAt, p.AbortReason,
		  p.ID,
	   )
	   if err != nil {
		   return fmt.Errorf("restoregate.SQLStore.UpdatePlan: %w", err)
	  }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrPlanNotFound
	   }
	  return nil
  }

  func (s *SQLStore) ListPlans(ctx context.Context, limit int) ([]PlanSummary, error) {
	   if limit <= 0 {
		   limit = 20
	  }
	   q := `SELECT id, blob_id, state, target_gen, creator, created_at, updated_at
		FROM restore_plans ORDER BY created_at DESC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, limit)
	  if err != nil {
		   return nil, fmt.Errorf("restoregate.SQLStore.ListPlans: %w", err)
	  }
	   defer rows.Close()
	   var out []PlanSummary
	  for rows.Next() {
		   var ps PlanSummary
		  var state string
		   if err := rows.Scan(&ps.ID, &ps.BlobID, &state, &ps.TargetGen, &ps.Creator, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			  return nil, fmt.Errorf("restoregate.SQLStore.ListPlans scan: %w", err)
		   }
		   ps.State = PlanState(state)
		  out = append(out, ps)
	   }
	  return out, rows.Err()
   }

  func (s *SQLStore) ListActivePlans(ctx context.Context) ([]RestorePlan, error) {
	  q := `SELECT id FROM restore_plans WHERE state IN ('freezing_writes','importing','verifying')`
	  rows, err := s.db.QueryContext(ctx, q)
	   if err != nil {
		   return nil, fmt.Errorf("restoregate.SQLStore.ListActivePlans: %w", err)
	   }
	   defer rows.Close()
	   var ids []string
	  for rows.Next() {
		   var id string
		  if err := rows.Scan(&id); err != nil {
			   return nil, fmt.Errorf("restoregate.SQLStore.ListActivePlans scan: %w", err)
		   }
		  ids = append(ids, id)
	   }
	  if err := rows.Err(); err != nil {
		  return nil, err
	   }
	  var plans []RestorePlan
	   for _, id := range ids {
		  p, err := s.GetPlan(ctx, id)
		   if err != nil {
			  continue
		   }
		   plans = append(plans, p)
	   }
	  return plans, nil
  }

// ── GenGate ─────────────────────────────────────────────────────────────────

   func (s *SQLStore) CreateGenGate(ctx context.Context, g GenGate) error {
	   q := `INSERT INTO restore_gens
		(id, plan_id, target_gen, system_gen, live_watermark, allowed, deny_reason, created_at)
		VALUES (?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q,
		  g.ID,
		  g.PlanID, g.TargetGen, g.SystemGen, g.LiveWatermark,
		   boolToInt(g.Allowed), g.DenyReason, g.CreatedAt,
	  )
	  if err != nil {
		  return fmt.Errorf("restoregate.SQLStore.CreateGenGate: %w", err)
	  }
	  return nil
   }

   func (s *SQLStore) ListGenGates(ctx context.Context, planID string) ([]GenGate, error) {
	   q := `SELECT id, plan_id, target_gen, system_gen, live_watermark, allowed, deny_reason, created_at 
		FROM restore_gens WHERE plan_id=? ORDER BY created_at`
	   rows, err := s.db.QueryContext(ctx, q, planID)
	   if err != nil {
		   return nil, fmt.Errorf("restoregate.SQLStore.ListGenGates: %w", err)
	   }
	   defer rows.Close()
	   var out []GenGate
	   for rows.Next() {
		   var g GenGate
		   var allowed int
		  if err := rows.Scan(&g.ID, &g.PlanID, &g.TargetGen, &g.SystemGen, &g.LiveWatermark, &allowed, &g.DenyReason, &g.CreatedAt); err != nil {
			   return nil, fmt.Errorf("restoregate.SQLStore.ListGenGates scan: %w", err)
		  }
		   g.Allowed = allowed != 0
		  out = append(out, g)
	   }
	  return out, rows.Err()
   }

// ── WriteMutex ───────────────────────────────────────────────────────────────

   func (s *SQLStore) CreateWriteMutex(ctx context.Context, m WriteMutex) error {
	   q := `INSERT INTO write_mutexes
		(id, plan_id, state, holder_gen, expires_at, acquired_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		  m.ID,
		   m.PlanID, string(m.State), m.HolderGen,
		  m.ExpiresAt, m.AcquiredAt, m.CreatedAt, m.UpdatedAt,
	   )
	   if err != nil {
		  return fmt.Errorf("restoregate.SQLStore.CreateWriteMutex: %w", err)
	  }
	   return nil
  }

   func (s *SQLStore) GetActiveMutex(ctx context.Context) (WriteMutex, error) {
	   q := `SELECT id, plan_id, state, holder_gen, expires_at, acquired_at, released_at, created_at, updated_at
		FROM write_mutexes WHERE state='active' ORDER BY created_at DESC LIMIT 1`
	  return s.scanMutex(s.db.QueryRowContext(ctx, q))
   }

  func (s *SQLStore) GetMutexByPlan(ctx context.Context, planID string) (WriteMutex, error) {
	   q := `SELECT id, plan_id, state, holder_gen, expires_at, acquired_at, released_at, created_at, updated_at
		FROM write_mutexes WHERE plan_id=? ORDER BY created_at DESC LIMIT 1`
	  return s.scanMutex(s.db.QueryRowContext(ctx, q, planID))
  }

  func (s *SQLStore) scanMutex(row *sql.Row) (WriteMutex, error) {
	  var m WriteMutex
	  var state string
	   if err := row.Scan(&m.ID, &m.PlanID, &state, &m.HolderGen, &m.ExpiresAt, &m.AcquiredAt, &m.ReleasedAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return WriteMutex{}, ErrMutexNotFound
		  }
		  return WriteMutex{}, fmt.Errorf("restoregate.SQLStore.scanMutex: %w", err)
	   }
	   m.State = WriteMutexState(state)
	  return m, nil
  }

   func (s *SQLStore) UpdateMutex(ctx context.Context, m WriteMutex) error {
	   q := `UPDATE write_mutexes SET state=?, expires_at=?, released_at=?, updated_at=? WHERE id=?`
	   res, err := s.db.ExecContext(ctx, q, string(m.State), m.ExpiresAt, m.ReleasedAt, m.UpdatedAt, m.ID)
	  if err != nil {
		  return fmt.Errorf("restoregate.SQLStore.UpdateMutex: %w", err)
	  }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrMutexNotFound
	  }
	  return nil
  }

   func (s *SQLStore) AppendBlockedWrite(ctx context.Context, bw BlockedWrite) error {
	  q := `INSERT INTO blocked_writes (id, mutex_id, plan_id, kind, user_id, asset, amount, reason, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		  bw.ID,
		  bw.MutexID, bw.PlanID, bw.Kind, bw.UserID, bw.Asset, bw.Amount, bw.Reason, bw.CreatedAt,
	   )
	  if err != nil {
		   return fmt.Errorf("restoregate.SQLStore.AppendBlockedWrite: %w", err)
	   }
	  return nil
   }

   func (s *SQLStore) ListBlockedWrites(ctx context.Context, mutexID string, limit int) ([]BlockedWrite, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   q := `SELECT id, mutex_id, plan_id, kind, user_id, asset, amount, reason, created_at
		FROM blocked_writes WHERE mutex_id=? ORDER BY created_at LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, mutexID, limit)
	   if err != nil {
		  return nil, fmt.Errorf("restoregate.SQLStore.ListBlockedWrites: %w", err)
	   }
	   defer rows.Close()
	  var out []BlockedWrite
	   for rows.Next() {
		  var bw BlockedWrite
		   if err := rows.Scan(&bw.ID, &bw.MutexID, &bw.PlanID, &bw.Kind, &bw.UserID, &bw.Asset, &bw.Amount, &bw.Reason, &bw.CreatedAt); err != nil {
			   return nil, fmt.Errorf("restoregate.SQLStore.ListBlockedWrites scan: %w", err)
		   }
		  out = append(out, bw)
	   }
	  return out, rows.Err()
   }

// ── HalfSeal ─────────────────────────────────────────────────────────────────

   func (s *SQLStore) CreateHalfSeal(ctx context.Context, hs HalfSeal) error {
	   keysJSON, _ := json.Marshal(hs.WrittenKeys)
	  q := `INSERT INTO half_seals
		(id, plan_id, written_keys, failed_at, fail_reason, resumed, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE id=id` // 幂等：相同 planID 已存在时静默忽略（首条封印优先）
	   _, err := s.db.ExecContext(ctx, q,
		   hs.ID, hs.PlanID, string(keysJSON), hs.FailedAt,
		  hs.FailReason, boolToInt(hs.Resumed), hs.CreatedAt, hs.UpdatedAt,
	  )
	   if err != nil {
		  if isDuplicateKey(err) {
			   return ErrHalfSealExists
		  }
		  return fmt.Errorf("restoregate.SQLStore.CreateHalfSeal: %w", err)
	  }
	  return nil
   }

  func (s *SQLStore) GetHalfSeal(ctx context.Context, planID string) (HalfSeal, error) {
	   q := `SELECT id, plan_id, written_keys, failed_at, fail_reason, resumed, resumed_at, created_at, updated_at
		FROM half_seals WHERE plan_id=? ORDER BY created_at DESC LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, planID)
	  var hs HalfSeal
	   var keysJSON string
	   var resumed int
	  if err := row.Scan(&hs.ID, &hs.PlanID, &keysJSON, &hs.FailedAt, &hs.FailReason, &resumed, &hs.ResumedAt, &hs.CreatedAt, &hs.UpdatedAt); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return HalfSeal{}, ErrHalfSealNotFound
		  }
		  return HalfSeal{}, fmt.Errorf("restoregate.SQLStore.GetHalfSeal: %w", err)
	  }
	  hs.Resumed = resumed != 0
	   _ = json.Unmarshal([]byte(keysJSON), &hs.WrittenKeys)
	  return hs, nil
  }

  func (s *SQLStore) UpdateHalfSeal(ctx context.Context, hs HalfSeal) error {
	  q := `UPDATE half_seals SET resumed=?, resumed_at=?, updated_at=? WHERE plan_id=?`
	  res, err := s.db.ExecContext(ctx, q, boolToInt(hs.Resumed), hs.ResumedAt, hs.UpdatedAt, hs.PlanID)
	  if err != nil {
		   return fmt.Errorf("restoregate.SQLStore.UpdateHalfSeal: %w", err)
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrHalfSealNotFound
	   }
	   return nil
   }

// ── SplitBrainProof ──────────────────────────────────────────────────────────

   func (s *SQLStore) CreateSplitBrainProof(ctx context.Context, p SplitBrainProof) error {
	  q := `INSERT INTO splitbrain_proofs
		(id, plan_id, balance_root_pre, balance_root_post, balance_root_target, diff_summary,
		 state, opened_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		   p.ID, p.PlanID, p.BalanceRootPre, p.BalanceRootPost, p.BalanceRootTarget,
		   p.DiffSummary, string(p.State), p.OpenedAt, p.CreatedAt, p.UpdatedAt,
	  )
	  if err != nil {
		   return fmt.Errorf("restoregate.SQLStore.CreateSplitBrainProof: %w", err)
	  }
	  return nil
   }

  func (s *SQLStore) GetSplitBrainProof(ctx context.Context, id string) (SplitBrainProof, error) {
	   q := `SELECT id, plan_id, balance_root_pre, balance_root_post, balance_root_target, diff_summary,
		state, opened_at, cleared_at, created_at, updated_at
		FROM splitbrain_proofs WHERE id=?`
	   row := s.db.QueryRowContext(ctx, q, id)
	   p, err := s.scanProof(row)
	  if err != nil {
		   return SplitBrainProof{}, err
	  }
	   p.Approvals, _ = s.listApprovals(ctx, id)
	  return p, nil
  }

  func (s *SQLStore) GetOpenProofByPlan(ctx context.Context, planID string) (SplitBrainProof, error) {
	  q := `SELECT id, plan_id, balance_root_pre, balance_root_post, balance_root_target, diff_summary,
		state, opened_at, cleared_at, created_at, updated_at
		FROM splitbrain_proofs WHERE plan_id=? AND state IN ('open','clearing')
		ORDER BY created_at DESC LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, planID)
	   p, err := s.scanProof(row)
	   if err != nil {
		  return SplitBrainProof{}, err
	  }
	  p.Approvals, _ = s.listApprovals(ctx, p.ID)
	   return p, nil
   }

  func (s *SQLStore) scanProof(row *sql.Row) (SplitBrainProof, error) {
	  var p SplitBrainProof
	  var state string
	  if err := row.Scan(
		   &p.ID, &p.PlanID, &p.BalanceRootPre, &p.BalanceRootPost, &p.BalanceRootTarget,
		  &p.DiffSummary, &state, &p.OpenedAt, &p.ClearedAt, &p.CreatedAt, &p.UpdatedAt,
	   ); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return SplitBrainProof{}, ErrSplitBrainNotFound
		   }
		  return SplitBrainProof{}, fmt.Errorf("restoregate.SQLStore.scanProof: %w", err)
	  }
	  p.State = SplitBrainState(state)
	  return p, nil
   }

   func (s *SQLStore) UpdateSplitBrainProof(ctx context.Context, p SplitBrainProof) error {
	   q := `UPDATE splitbrain_proofs SET state=?, diff_summary=?, cleared_at=?, updated_at=? WHERE id=?`
	   res, err := s.db.ExecContext(ctx, q, string(p.State), p.DiffSummary, p.ClearedAt, p.UpdatedAt, p.ID)
	   if err != nil {
		  return fmt.Errorf("restoregate.SQLStore.UpdateSplitBrainProof: %w", err)
	  }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrSplitBrainNotFound
	   }
	  return nil
  }

  func (s *SQLStore) ListOpenProofs(ctx context.Context) ([]SplitBrainProof, error) {
	   q := `SELECT id, plan_id, balance_root_pre, balance_root_post, balance_root_target, diff_summary,
		state, opened_at, cleared_at, created_at, updated_at
		FROM splitbrain_proofs WHERE state IN ('open','clearing') ORDER BY opened_at`
	   rows, err := s.db.QueryContext(ctx, q)
	   if err != nil {
		   return nil, fmt.Errorf("restoregate.SQLStore.ListOpenProofs: %w", err)
	  }
	  defer rows.Close()
	  var out []SplitBrainProof
	  for rows.Next() {
		   var p SplitBrainProof
		   var state string
		   if err := rows.Scan(
			   &p.ID, &p.PlanID, &p.BalanceRootPre, &p.BalanceRootPost, &p.BalanceRootTarget,
			   &p.DiffSummary, &state, &p.OpenedAt, &p.ClearedAt, &p.CreatedAt, &p.UpdatedAt,
		  ); err != nil {
			   return nil, fmt.Errorf("restoregate.SQLStore.ListOpenProofs scan: %w", err)
		  }
		   p.State = SplitBrainState(state)
		   out = append(out, p)
	   }
	  return out, rows.Err()
  }

  func (s *SQLStore) AppendProofApproval(ctx context.Context, a ProofApproval) error {
	  q := `INSERT INTO proof_approvals (id, proof_id, approver, role, comment, created_at)
		VALUES (?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		  uuid.New().String(), a.ProofID, a.Approver, a.Role, a.Comment, a.CreatedAt,
	  )
	   if err != nil {
		  return fmt.Errorf("restoregate.SQLStore.AppendProofApproval: %w", err)
	   }
	   return nil
   }

   func (s *SQLStore) listApprovals(ctx context.Context, proofID string) ([]ProofApproval, error) {
	   q := `SELECT proof_id, approver, role, comment, created_at FROM proof_approvals WHERE proof_id=? ORDER BY created_at`
	  rows, err := s.db.QueryContext(ctx, q, proofID)
	  if err != nil {
		  return nil, fmt.Errorf("restoregate.SQLStore.listApprovals: %w", err)
	   }
	   defer rows.Close()
	   var out []ProofApproval
	   for rows.Next() {
		  var a ProofApproval
		   if err := rows.Scan(&a.ProofID, &a.Approver, &a.Role, &a.Comment, &a.CreatedAt); err != nil {
			   return nil, fmt.Errorf("restoregate.SQLStore.listApprovals scan: %w", err)
		  }
		  out = append(out, a)
	   }
	   return out, rows.Err()
  }

// ── RestoreGenRecord ──────────────────────────────────────────────────────────

  func (s *SQLStore) GetGenRecord(ctx context.Context) (RestoreGenRecord, error) {
	   q := `SELECT id, current_gen, updated_at FROM restore_gen_record WHERE id='global' LIMIT 1`
	  var r RestoreGenRecord
	   if err := s.db.QueryRowContext(ctx, q).Scan(&r.ID, &r.CurrentGen, &r.UpdatedAt); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return RestoreGenRecord{ID: "global", CurrentGen: 0, UpdatedAt: s.clock()}, nil
		  }
		  return RestoreGenRecord{}, fmt.Errorf("restoregate.SQLStore.GetGenRecord: %w", err)
	  }
	  return r, nil
  }

  func (s *SQLStore) UpsertGenRecord(ctx context.Context, r RestoreGenRecord) error {
	   q := `INSERT INTO restore_gen_record (id, current_gen, updated_at)
		VALUES ('global',?,?)
		ON DUPLICATE KEY UPDATE current_gen=VALUES(current_gen), updated_at=VALUES(updated_at)`
	   _, err := s.db.ExecContext(ctx, q, r.CurrentGen, r.UpdatedAt)
	  if err != nil {
		   return fmt.Errorf("restoregate.SQLStore.UpsertGenRecord: %w", err)
	  }
	  return nil
  }

// ── helpers ──────────────────────────────────────────────────────────────────

   func boolToInt(b bool) int {
	  if b {
		   return 1
	  }
	   return 0
   }

  func isDuplicateKey(err error) bool {
	  if err == nil {
		   return false
	   }
	  msg := err.Error()
	  return strings.Contains(msg, "Duplicate entry") ||
		  strings.Contains(msg, "duplicate key") ||
		   strings.Contains(msg, "UNIQUE constraint failed")
  }
