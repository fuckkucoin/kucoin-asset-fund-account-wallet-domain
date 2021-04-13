package dispclaw

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Store SQL 持久化接口。进程注入 *sql.DB；注入 nil 退化到内存 Service。
type Store struct {
	db *sql.DB
}

// NewStore 构造 SQL 存储适配器。

   func NewStore(db *sql.DB) *Store {
	   return &Store{db: db}
  }

// ─────────────────────────── FundPlan ───────────────────────────
// SavePlan 插入或更新 FundPlan（按 version 乐观锁）。
  func (st *Store) SavePlan(ctx context.Context, p FundPlan) error {
	   if st.db == nil {
		  return nil
	  }
	  query := `
		INSERT INTO dispute_fund_plans
			(id, case_id, channel, user_id, asset, amount, plan_type, status,
			 rail_break_id, opened_by, closed_by, abort_reason, version,
			 fence_epoch, fence_token, created_at, updated_at)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status=VALUES(status), closed_by=VALUES(closed_by),
			abort_reason=VALUES(abort_reason), version=VALUES(version)+1,
			fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token),
			updated_at=VALUES(updated_at)
	`
	  _,
		  err := st.db.ExecContext(ctx, query,
		  p.ID, p.CaseID, p.Channel, p.UserID, p.Asset, p.Amount,
		   string(p.PlanType), string(p.Status),
		  p.RailBreakID, p.OpenedBy, p.ClosedBy, p.AbortReason, p.Version,
		  p.FenceEpoch, p.FenceToken,
		  p.CreatedAt.UTC(), p.UpdatedAt.UTC(),
	  )
	  return err
   }

// LoadPlanByCase 按 case_id 查询 FundPlan。

   func (st *Store) LoadPlanByCase(ctx context.Context, caseID string) (FundPlan, error) {
	   if st.db == nil {
		   return FundPlan{}, ErrPlan
	   }
	   row := st.db.QueryRowContext(ctx, `
		SELECT id, case_id, channel, user_id, asset, amount, plan_type, status,
		       rail_break_id, opened_by, closed_by, abort_reason, version,
		       fence_epoch, fence_token, created_at, updated_at
		FROM dispute_fund_plans WHERE case_id=?`, caseID)
	   return scanPlan(row)
   }

// LoadPlan 按 plan_id 查询 FundPlan。

  func (st *Store) LoadPlan(ctx context.Context, planID string) (FundPlan, error) {
	   if st.db == nil {
		   return FundPlan{}, ErrPlan
	   }
	  row := st.db.QueryRowContext(ctx, `
		SELECT id, case_id, channel, user_id, asset, amount, plan_type, status,
		       rail_break_id, opened_by, closed_by, abort_reason, version,
		       fence_epoch, fence_token, created_at, updated_at
		FROM dispute_fund_plans WHERE id=?`, planID)
	   return scanPlan(row)
  }

// ListOpenPlans 列出所有非终态计划（worker 扫描用）。
   func (st *Store) ListOpenPlans(ctx context.Context) ([]FundPlan, error) {
	  if st.db == nil {
		   return nil, nil
	  }
	   rows, err := st.db.QueryContext(ctx, `
		SELECT id, case_id, channel, user_id, asset, amount, plan_type, status,
		       rail_break_id, opened_by, closed_by, abort_reason, version,
		       fence_epoch, fence_token, created_at, updated_at
		FROM dispute_fund_plans
		WHERE status NOT IN ('closed','aborted')
		ORDER BY created_at ASC LIMIT 500`)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []FundPlan
	   for rows.Next() {
		  p, err := scanPlanRow(rows)
		  if err != nil {
			   return nil, err
		  }
		   out = append(out, p)
	  }
	   return out, rows.Err()
  }

  func scanPlan(row *sql.Row) (FundPlan, error) {
	   var p FundPlan
	  var planType, status string
	   err := row.Scan(
		  &p.ID, &p.CaseID, &p.Channel, &p.UserID, &p.Asset, &p.Amount,
		   &planType, &status,
		   &p.RailBreakID, &p.OpenedBy, &p.ClosedBy, &p.AbortReason, &p.Version,
		  &p.FenceEpoch, &p.FenceToken, &p.CreatedAt, &p.UpdatedAt,
	   )
	   if err == sql.ErrNoRows {
		   return FundPlan{}, ErrPlan
	  }
	   if err != nil {
		  return FundPlan{}, fmt.Errorf("dispclaw: scan plan: %w", err)
	   }
	   p.PlanType = PlanType(planType)
	  p.Status = PlanStatus(status)
	   return p, nil
  }

  func scanPlanRow(rows *sql.Rows) (FundPlan, error) {
	  var p FundPlan
	   var planType, status string
	  err := rows.Scan(
		   &p.ID, &p.CaseID, &p.Channel, &p.UserID, &p.Asset, &p.Amount,
		  &planType, &status,
		   &p.RailBreakID, &p.OpenedBy, &p.ClosedBy, &p.AbortReason, &p.Version,
		   &p.FenceEpoch, &p.FenceToken, &p.CreatedAt, &p.UpdatedAt,
	   )
	  if err != nil {
		  return FundPlan{}, fmt.Errorf("dispclaw: scan plan row: %w", err)
	   }
	  p.PlanType = PlanType(planType)
	   p.Status = PlanStatus(status)
	   return p, nil
   }

// ─────────────────────────── RecoveryLeg ────────────────────────────────────
// SaveLeg 插入或更新 RecoveryLeg（幂等键唯一索引保证）。

  func (st *Store) SaveLeg(ctx context.Context, l RecoveryLeg) error {
	   if st.db == nil {
		  return nil
	   }
	  var postedAt, failedAt, nextRetryAt sql.NullTime
	   if l.PostedAt != nil {
		   postedAt = sql.NullTime{Time: *l.PostedAt,
			   Valid: true}
	  }
	   if l.FailedAt != nil {
		  failedAt = sql.NullTime{Time: *l.FailedAt, Valid: true}
	   }
	  if l.NextRetryAt != nil {
		   nextRetryAt = sql.NullTime{Time: *l.NextRetryAt, Valid: true}
	  }
	   query := `
		INSERT INTO recovery_legs
			(id, plan_id, case_id, seq, leg_type, user_id, asset, amount, status,
			 idempotency_key, ledger_ref, hold_ref, rail_break_ref, leg_hash,
			 posted_by, note, posted_at, failed_at, fail_reason,
			 retry_count, next_retry_at, fence_epoch, fence_token, created_at, updated_at)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status=VALUES(status), posted_by=VALUES(posted_by),
			note=VALUES(note), posted_at=VALUES(posted_at),
			failed_at=VALUES(failed_at), fail_reason=VALUES(fail_reason),
			retry_count=VALUES(retry_count), next_retry_at=VALUES(next_retry_at),
			leg_hash=VALUES(leg_hash),
			fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token),
			updated_at=VALUES(updated_at)
	`
	  _, err := st.db.ExecContext(ctx, query,
		   l.ID,
		   l.PlanID, l.CaseID, l.Seq, string(l.LegType),
		   l.UserID,
		  l.Asset, l.Amount, string(l.Status),
		  l.IdempotencyKey, l.LedgerRef, l.HoldRef, l.RailBreakRef, l.LegHash,
		   l.PostedBy, l.Note, postedAt, failedAt, l.FailReason,
		   l.RetryCount, nextRetryAt, l.FenceEpoch, l.FenceToken,
		   l.CreatedAt.UTC(), l.UpdatedAt.UTC(),
	   )
	   return err
   }

// LoadLegsByPlan 查询 FundPlan 下所有追偿腿（按 seq 升序）。
  func (st *Store) LoadLegsByPlan(ctx context.Context, planID string) ([]RecoveryLeg, error) {
	   if st.db == nil {
		  return nil, nil
	  }
	   rows, err := st.db.QueryContext(ctx, `
		SELECT id, plan_id, case_id, seq, leg_type, user_id, asset, amount, status,
		       idempotency_key, ledger_ref, hold_ref, rail_break_ref, leg_hash,
		       posted_by, note, posted_at, failed_at, fail_reason,
		       retry_count, next_retry_at, fence_epoch, fence_token, created_at, updated_at
		FROM recovery_legs WHERE plan_id=? ORDER BY seq ASC`, planID)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []RecoveryLeg
	   for rows.Next() {
		   l, err := scanLegRow(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, l)
	  }
	  return out, rows.Err()
  }

// LoadPendingLegs 查询所有 pending 状态且到了重试时间的腿（worker 用）。

   func (st *Store) LoadPendingLegs(ctx context.Context,
	   now time.Time) ([]RecoveryLeg, error) {
	  if st.db == nil {
		   return nil, nil
	  }
	  rows, err := st.db.QueryContext(ctx, `
		SELECT id, plan_id, case_id, seq, leg_type, user_id, asset, amount, status,
		       idempotency_key, ledger_ref, hold_ref, rail_break_ref, leg_hash,
		       posted_by, note, posted_at, failed_at, fail_reason,
		       retry_count, next_retry_at, fence_epoch, fence_token, created_at, updated_at
		FROM recovery_legs
		WHERE status IN ('pending','failed')
		  AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY created_at ASC LIMIT 200`, now.UTC())
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []RecoveryLeg
	  for rows.Next() {
		  l, err := scanLegRow(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, l)
	   }
	  return out, rows.Err()
   }

   func scanLegRow(rows *sql.Rows) (RecoveryLeg, error) {
	  var l RecoveryLeg
	  var legType, status string
	   var postedAt, failedAt, nextRetryAt sql.NullTime
	  err := rows.Scan(
		   &l.ID, &l.PlanID, &l.CaseID, &l.Seq, &legType,
		   &l.UserID, &l.Asset, &l.Amount, &status,
		   &l.IdempotencyKey, &l.LedgerRef, &l.HoldRef, &l.RailBreakRef, &l.LegHash,
		   &l.PostedBy,
		  &l.Note, &postedAt, &failedAt, &l.FailReason,
		   &l.RetryCount, &nextRetryAt, &l.FenceEpoch, &l.FenceToken,
		   &l.CreatedAt, &l.UpdatedAt,
	  )
	  if err != nil {
		  return RecoveryLeg{}, fmt.Errorf("dispclaw: scan leg row: %w", err)
	  }
	  l.LegType = LegType(legType)
	   l.Status = LegStatus(status)
	   if postedAt.Valid {
		  t := postedAt.Time
		  l.PostedAt = &t
	   }
	  if failedAt.Valid {
		  t := failedAt.Time
		   l.FailedAt = &t
	   }
	  if nextRetryAt.Valid {
		   t := nextRetryAt.Time
		  l.NextRetryAt = &t
	  }
	   return l, nil
   }

// ─────────────────────────── CloseGate ──────────────────────────────────────
// SaveGate 插入或更新 CloseGate。
   func (st *Store) SaveGate(ctx context.Context, g CloseGate) error {
	   if st.db == nil {
		   return nil
	   }
	  var passedAt sql.NullTime
	   if g.PassedAt != nil {
		   passedAt = sql.NullTime{Time: *g.PassedAt,
			   Valid: true}
	  }
	   query := `
		INSERT INTO close_gates
			(id, plan_id, case_id, gate_status, blocking_reason, pending_leg_ids,
			 checked_by, passed_at, bypass_reason, bypassed_by, version, created_at, updated_at)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			gate_status=VALUES(gate_status),
			blocking_reason=VALUES(blocking_reason),
			pending_leg_ids=VALUES(pending_leg_ids),
			checked_by=VALUES(checked_by),
			passed_at=VALUES(passed_at),
			bypass_reason=VALUES(bypass_reason),
			bypassed_by=VALUES(bypassed_by),
			version=VALUES(version)+1,
			updated_at=VALUES(updated_at)
	`
	  _, err := st.db.ExecContext(ctx, query,
		  g.ID, g.PlanID, g.CaseID, string(g.GateStatus),
		   g.BlockingReason, g.PendingLegIDs,
		   g.CheckedBy, passedAt, g.BypassReason, g.BypassedBy, g.Version,
		  g.CreatedAt.UTC(), g.UpdatedAt.UTC(),
	  )
	   return err
  }

// LoadGateByCaseID 按 case_id 查询 CloseGate。

  func (st *Store) LoadGateByCaseID(ctx context.Context, caseID string) (CloseGate, error) {
	   if st.db == nil {
		  return CloseGate{}, ErrGate
	   }
	  row := st.db.QueryRowContext(ctx, ` 
		SELECT id, plan_id, case_id, gate_status, blocking_reason, pending_leg_ids,
		       checked_by, passed_at, bypass_reason, bypassed_by, version, created_at, updated_at
		FROM close_gates WHERE case_id=?`, caseID)
	   var g CloseGate
	   var gateStatus string
	  var passedAt sql.NullTime
	  err := row.Scan(
		  &g.ID, &g.PlanID, &g.CaseID, &gateStatus,
		   &g.BlockingReason, &g.PendingLegIDs,
		   &g.CheckedBy, &passedAt, &g.BypassReason, &g.BypassedBy, &g.Version,
		  &g.CreatedAt, &g.UpdatedAt,
	   )
	   if err == sql.ErrNoRows {
		  return CloseGate{}, ErrGate
	   }
	   if err != nil {
		   return CloseGate{}, fmt.Errorf("dispclaw: scan gate: %w", err)
	  }
	  g.GateStatus = GateStatus(gateStatus)
	  if passedAt.Valid {
		   t := passedAt.Time
		   g.PassedAt = &t
	  }
	   return g, nil
   }

// LoadBlockingGates 列出所有 blocking 状态的收口闸（worker 巡检用）。
   func (st *Store) LoadBlockingGates(ctx context.Context) ([]CloseGate, error) {
	   if st.db == nil {
		   return nil, nil
	  }
	  rows, err := st.db.QueryContext(ctx, `
		SELECT id, plan_id, case_id, gate_status, blocking_reason, pending_leg_ids,
		       checked_by, passed_at, bypass_reason, bypassed_by, version, created_at, updated_at
		FROM close_gates WHERE gate_status='blocking'
		ORDER BY created_at ASC LIMIT 500`)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []CloseGate
	   for rows.Next() {
		   var g CloseGate
		   var gateStatus string
		  var passedAt sql.NullTime
		  if err := rows.Scan(
			  &g.ID, &g.PlanID, &g.CaseID, &gateStatus,
			   &g.BlockingReason,
			  &g.PendingLegIDs,
			  &g.CheckedBy, &passedAt, &g.BypassReason, &g.BypassedBy, &g.Version,
			   &g.CreatedAt, &g.UpdatedAt,
		  ); err != nil {
			  return nil, fmt.Errorf("dispclaw: scan gate row: %w", err)
		   }
		  g.GateStatus = GateStatus(gateStatus)
		   if passedAt.Valid {
			   t := passedAt.Time
			   g.PassedAt = &t
		  }
		  out = append(out, g)
	   }
	   return out, rows.Err()
   }

// ─────────────────────────── CloseProof ─────────────────────────────────────
// SaveProof 插入收口证明（仅插入，不允许更新——证明一旦生成禁止修改）。

   func (st *Store) SaveProof(ctx context.Context, p CloseProof) error {
	   if st.db == nil {
		   return nil
	   }
	  var tamperCheckedAt sql.NullTime
	   if p.TamperCheckedAt != nil {
		   tamperCheckedAt = sql.NullTime{Time: *p.TamperCheckedAt, Valid: true}
	   }
	   _, err := st.db.ExecContext(ctx, ` 
		INSERT IGNORE INTO close_proofs
			(id, plan_id, case_id, gate_id, proof_hash, leg_hashes, leg_count,
			 total_amount, asset, decided_by, decision,
			 tamper_detected, tamper_detail, tamper_checked_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		   p.ID, p.PlanID, p.CaseID, p.GateID,
		  p.ProofHash, p.LegHashes, p.LegCount,
		  p.TotalAmount, p.Asset, p.DecidedBy, p.Decision,
		   p.TamperDetected, p.TamperDetail, tamperCheckedAt,
		   p.CreatedAt.UTC(),
	   )
	   return err
   }

// UpdateProofTamper 更新收口证明篡改检测标记（巡检 worker 调用）。

   func (st *Store) UpdateProofTamper(ctx context.Context, proofID string, detected bool, detail string, checkedAt time.Time) error {
	   if st.db == nil {
		  return nil
	  }
	  _, err := st.db.ExecContext(ctx, `
		UPDATE close_proofs SET
			tamper_detected=?, tamper_detail=?, tamper_checked_at=?
		WHERE id=?`,
		  detected, detail, checkedAt.UTC(), proofID,
	   )
	   return err
   }

// LoadProofByCaseID 按 case_id 查询 CloseProof。
  func (st *Store) LoadProofByCaseID(ctx context.Context, caseID string) (CloseProof, error) {
	   if st.db == nil {
		  return CloseProof{}, ErrProof
	  }
	   row := st.db.QueryRowContext(ctx, `
		SELECT id, plan_id, case_id, gate_id, proof_hash, leg_hashes, leg_count,
		       total_amount, asset, decided_by, decision,
		       tamper_detected, tamper_detail, tamper_checked_at, created_at
		FROM close_proofs WHERE case_id=?`, caseID)
	   return scanProof(row)
   }

// LoadAllProofs 列出所有收口证明（巡检 worker 用）。

   func (st *Store) LoadAllProofs(ctx context.Context) ([]CloseProof, error) {
	  if st.db == nil {
		   return nil, nil
	  }
	  rows, err := st.db.QueryContext(ctx, `
		SELECT id, plan_id, case_id, gate_id, proof_hash, leg_hashes, leg_count,
		       total_amount, asset, decided_by, decision,
		       tamper_detected, tamper_detail, tamper_checked_at, created_at
		FROM close_proofs ORDER BY created_at DESC LIMIT 1000`)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []CloseProof
	   for rows.Next() {
		   p, err := scanProofRow(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, p)
	   }
	   return out, rows.Err()
  }

  func scanProof(row *sql.Row) (CloseProof, error) {
	   var p CloseProof
	  var tamperCheckedAt sql.NullTime
	   var tamperDetected int
	  err := row.Scan(
		  &p.ID, &p.PlanID, &p.CaseID, &p.GateID,
		  &p.ProofHash, &p.LegHashes, &p.LegCount,
		   &p.TotalAmount, &p.Asset, &p.DecidedBy, &p.Decision,
		  &tamperDetected, &p.TamperDetail, &tamperCheckedAt, &p.CreatedAt,
	  )
	   if err == sql.ErrNoRows {
		   return CloseProof{}, ErrProof
	  }
	   if err != nil {
		   return CloseProof{}, fmt.Errorf("dispclaw: scan proof: %w", err)
	   }
	  p.TamperDetected = tamperDetected != 0
	  if tamperCheckedAt.Valid {
		  t := tamperCheckedAt.Time
		  p.TamperCheckedAt = &t
	   }
	   return p, nil
  }

  func scanProofRow(rows *sql.Rows) (CloseProof, error) {
	   var p CloseProof
	   var tamperCheckedAt sql.NullTime
	   var tamperDetected int
	   err := rows.Scan(
		   &p.ID, &p.PlanID, &p.CaseID, &p.GateID,
		   &p.ProofHash,
		  &p.LegHashes, &p.LegCount,
		  &p.TotalAmount, &p.Asset, &p.DecidedBy, &p.Decision,
		  &tamperDetected, &p.TamperDetail, &tamperCheckedAt, &p.CreatedAt,
	   )
	  if err != nil {
		  return CloseProof{}, fmt.Errorf("dispclaw: scan proof row: %w", err)
	   }
	   p.TamperDetected = tamperDetected != 0
	   if tamperCheckedAt.Valid {
		   t := tamperCheckedAt.Time
		   p.TamperCheckedAt = &t
	  }
	  return p, nil
  }

// ─────────────────────────── 事务辅助 ───────────────────────────────────────
// WithTx 在事务中执行 fn；fn 返回错误时自动回滚。
  func (st *Store) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	   if st.db == nil {
		   return nil
	   }
	  tx,
		   err := st.db.BeginTx(ctx, nil)
	   if err != nil {
		   return fmt.Errorf("dispclaw: begin tx: %w", err)
	  }
	   if err := fn(tx); err != nil {
		  _ = tx.Rollback()
		  return err
	   }
	  return tx.Commit()
   }

  // ─────────────────────────── 指标收集 ───────────────────────────────────────
   // CollectStats 收集各状态计划和腿的数量（Prometheus 指标上报用）。

  func (st *Store) CollectStats(ctx context.Context) (planStats, legStats map[string]int64, err error) {
	   planStats = make(map[string]int64)
	   legStats = make(map[string]int64)
	  if st.db == nil {
		   return planStats, legStats, nil
	  }
	   rows, err := st.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM dispute_fund_plans GROUP BY status`)
	  if err != nil {
		   return nil, nil, err
	  }
	  defer rows.Close()
	   for rows.Next() {
		  var s string
		   var n int64
		  if err := rows.Scan(&s, &n); err != nil {
			  return nil, nil, err
		  }
		   planStats[s] = n
	   }
	   if err := rows.Err(); err != nil {
		   return nil, nil, err
	   }
	   rows2, err := st.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM recovery_legs GROUP BY status`)
	  if err != nil {
		  return nil, nil, err
	   }
	   defer rows2.Close()
	  for rows2.Next() {
		   var s string
		  var n int64
		   if err := rows2.Scan(&s, &n); err != nil {
			   return nil, nil, err
		   }
		  legStats[s] = n
	  }
	  return planStats, legStats, rows2.Err()
   }

// ─────────────────────────── 冻结观察（holdcat 只读引用） ───────────────────
// UpdateLegHoldRef 更新腿的 holdcat 引用（只读挂钩，不改 holdcat 主路径）。

   func (st *Store) UpdateLegHoldRef(ctx context.Context, legID, holdRef string) error {
	  if st.db == nil {
		   return nil
	   }
	  _, err := st.db.ExecContext(ctx, `
		UPDATE recovery_legs SET hold_ref=?, updated_at=? WHERE id=?`,
		  strings.TrimSpace(holdRef), time.Now().UTC(), strings.TrimSpace(legID),
	  )
	  return err
  }

// UpdateLegRailBreakRef 更新腿的法币轨道断裂 ID（只读挂钩 E 包）。
   func (st *Store) UpdateLegRailBreakRef(ctx context.Context, legID, railBreakRef string) error {
	   if st.db == nil {
		   return nil
	  }
	   _, err := st.db.ExecContext(ctx, `
		UPDATE recovery_legs SET rail_break_ref=?, updated_at=? WHERE id=?`,
		   strings.TrimSpace(railBreakRef), time.Now().UTC(), strings.TrimSpace(legID),
	   )
	  return err
   }
