package sagaresidue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Store 本包全局存储接口（由 SQLStore 实现，可用 MemStore 替代）。

type Store interface {
	// ── ResidueRun ────────────────────────────────────────────────────────────
	SaveResidueRun(ctx context.Context, r *ResidueRun) error
	GetResidueRun(ctx context.Context, id string) (*ResidueRun, error)
	GetResidueRunBySagaRunID(ctx context.Context, sagaRunID string) (*ResidueRun, error)
	ListResidueRuns(ctx context.Context, req RunListReq) ([]ResidueRun, error)
	UpdateResidueRunState(ctx context.Context, id string, newState RunState, updatedAt time.Time) error
	// ── ResidueItem ───────────────────────────────────────────────────────────
	SaveResidueItem(ctx context.Context, item *ResidueItem) error
	ListResidueItems(ctx context.Context,
		residueRunID string) ([]ResidueItem, error)
	ConfirmResidueItems(ctx context.Context, ids []string, confirmedAt time.Time) error
	// ── ResidueReport ─────────────────────────────────────────────────────────
	SaveReport(ctx context.Context, report *ResidueReport) error
	GetLatestReport(ctx context.Context, sagaRunID string) (*ResidueReport, error)
	ListReports(ctx context.Context, sagaRunID string, limit int) ([]ResidueReport, error)
	// ── CloseoutPlan ──────────────────────────────────────────────────────────
	SavePlan(ctx context.Context, plan *CloseoutPlan) error
	GetPlan(ctx context.Context, id string) (*CloseoutPlan, error)
	GetActivePlan(ctx context.Context, residueRunID string) (*CloseoutPlan, error)
	// ── CloseoutLeg ───────────────────────────────────────────────────────────
	SaveLeg(ctx context.Context, leg *CloseoutLeg) error
	GetLeg(ctx context.Context, id string) (*CloseoutLeg, error)
	ListLegs(ctx context.Context, req LegListReq) ([]CloseoutLeg, error)
	// ── CloseProof ────────────────────────────────────────────────────────────
	SaveProof(ctx context.Context, proof *CloseProof) error
	GetProof(ctx context.Context,
		id string) (*CloseProof, error)
	GetProofByRunID(ctx context.Context, residueRunID string) (*CloseProof, error)
	// ── IdemBlock ─────────────────────────────────────────────────────────────
	BlockIdem(ctx context.Context, idemKey, reason string) error
	IsIdemBlocked(ctx context.Context, idemKey string) (bool, error)
}

// SQLStore Store 的 MySQL/Postgres 实现。
type SQLStore struct {
	db *persist.SQLDB
}

// NewSQLStore 构造 SQLStore。
   func NewSQLStore(db *persist.SQLDB) *SQLStore {
	  return &SQLStore{db: db}
  }

// ── ResidueRun ────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveResidueRun(ctx context.Context, r *ResidueRun) error {
	  if s == nil || s.db == nil {
		   return ErrStorageNil
	   }
	   stepsJSON, err := json.Marshal(r.PartialDoneSteps)
	   if err != nil {
		  return err
	   }
	  q := s.db.Conn(ctx)
	   _, err = q.ExecContext(ctx, `
INSERT INTO sr_residue_runs
  (id, saga_run_id, user_id, asset, partial_done_steps, partial_cause,
   state, severity, mutex_generation, closeout_plan_id, close_proof_id,
   escalated_at, closed_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), severity=VALUES(severity),
  mutex_generation=VALUES(mutex_generation),
  closeout_plan_id=VALUES(closeout_plan_id),
  close_proof_id=VALUES(close_proof_id),
  escalated_at=VALUES(escalated_at),
  closed_at=VALUES(closed_at),
  updated_at=VALUES(updated_at)`,
		   r.ID, r.SagaRunID, r.UserID, r.Asset, string(stepsJSON), r.PartialCause,
		  string(r.State), string(r.Severity), r.MutexGeneration,
		  nullableStr(r.CloseoutPlanID), nullableStr(r.CloseProofID),
		   nullableTime(r.EscalatedAt), nullableTime(r.ClosedAt),
		  r.CreatedAt, r.UpdatedAt,
	  )
	   return err
   }

  func (s *SQLStore) GetResidueRun(ctx context.Context, id string) (*ResidueRun, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	  }
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT id, saga_run_id, user_id, asset, partial_done_steps, partial_cause,
       state, severity, mutex_generation, closeout_plan_id, close_proof_id,
       escalated_at, closed_at, created_at, updated_at
FROM sr_residue_runs WHERE id=?`, id)
	  return scanResidueRun(row)
   }

   func (s *SQLStore) GetResidueRunBySagaRunID(ctx context.Context, sagaRunID string) (*ResidueRun, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStorageNil
	   }
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, ` 
SELECT id, saga_run_id, user_id, asset, partial_done_steps, partial_cause,
       state, severity, mutex_generation, closeout_plan_id, close_proof_id,
       escalated_at, closed_at, created_at, updated_at
FROM sr_residue_runs WHERE saga_run_id=?`, sagaRunID)
	   return scanResidueRun(row)
  }

   func (s *SQLStore) ListResidueRuns(ctx context.Context, req RunListReq) ([]ResidueRun, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	  }
	  limit := req.Limit
	  if limit <= 0 || limit > 500 {
		  limit = 100
	  }
	   where := []string{"1=1"}
	  args := []any{}
	   if req.SagaRunID != "" {
		  where = append(where, "saga_run_id=?")
		  args = append(args, req.SagaRunID)
	  }
	  if req.State != "" {
		   where = append(where, "state=?")
		   args = append(args, string(req.State))
	  }
	   if req.Severity != "" {
		   where = append(where, "severity=?")
		   args = append(args, string(req.Severity))
	  }
	  if req.UserID != "" {
		   where = append(where, "user_id=?")
		  args = append(args, req.UserID)
	   }
	   if req.After != "" {
		   where = append(where, "id > ?")
		   args = append(args, req.After)
	  }
	   args = append(args, limit)
	   q := s.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, fmt.Sprintf(`
SELECT id, saga_run_id, user_id, asset, partial_done_steps, partial_cause,
       state, severity, mutex_generation, closeout_plan_id, close_proof_id,
       escalated_at, closed_at, created_at, updated_at
FROM sr_residue_runs WHERE %s ORDER BY id ASC LIMIT ?`, strings.Join(where, " AND ")), args...)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []ResidueRun
	  for rows.Next() {
		   rr, err := scanResidueRunRow(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, *rr)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) UpdateResidueRunState(ctx context.Context, id string, newState RunState, updatedAt time.Time) error {
	  if s == nil || s.db == nil {
		  return ErrStorageNil
	   }
	  q := s.db.Conn(ctx)
	  _, err := q.ExecContext(ctx, `UPDATE sr_residue_runs SET state=?, updated_at=? WHERE id=?`,
		  string(newState), updatedAt, id)
	   return err
   }

// ── ResidueItem ───────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveResidueItem(ctx context.Context, item *ResidueItem) error {
	  if s == nil || s.db == nil {
		   return ErrStorageNil
	  }
	  q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx, `
INSERT INTO sr_residue_items
  (id, residue_run_id, step, idem_key, ledger_entry_id, amount, asset, direction,
   hop_state, is_confirmed, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  hop_state=VALUES(hop_state), is_confirmed=VALUES(is_confirmed)`,
		   item.ID, item.ResidueRunID, item.Step, item.IdemKey,
		  nullableStr(item.LedgerEntryID),
		   item.Amount, item.Asset,
		   item.Direction, item.HopState, item.IsConfirmed, item.CreatedAt,
	  )
	   return err
  }

  func (s *SQLStore) ListResidueItems(ctx context.Context, residueRunID string) ([]ResidueItem, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	   }
	   q := s.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx, `
SELECT id, residue_run_id, step, idem_key, ledger_entry_id, amount, asset, direction,
       hop_state, is_confirmed, created_at
FROM sr_residue_items WHERE residue_run_id=? ORDER BY step ASC`, residueRunID)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []ResidueItem
	  for rows.Next() {
		   var item ResidueItem
		  var ledgerID sql.NullString
		   if err := rows.Scan(&item.ID,
			   &item.ResidueRunID, &item.Step, &item.IdemKey,
			  &ledgerID,
			  &item.Amount, &item.Asset, &item.Direction,
			  &item.HopState, &item.IsConfirmed, &item.CreatedAt); err != nil {
			   return nil, err
		  }
		  item.LedgerEntryID = ledgerID.String
		  out = append(out, item)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) ConfirmResidueItems(ctx context.Context, ids []string, confirmedAt time.Time) error {
	  if s == nil || s.db == nil {
		   return ErrStorageNil
	   }
	  if len(ids) == 0 {
		  return nil
	  }
	   placeholders := make([]string, len(ids))
	  args := make([]any, len(ids))
	   for i, id := range ids {
		   placeholders[i] = "?"
		  args[i] = id
	  }
	   q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx, fmt.Sprintf(`UPDATE sr_residue_items SET is_confirmed=1 WHERE id IN (%s)`,
		   strings.Join(placeholders, ",")), args...)
	  return err
  }

// ── ResidueReport ─────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveReport(ctx context.Context, report *ResidueReport) error {
	  if s == nil || s.db == nil {
		  return ErrStorageNil
	   }
	   itemsJSON, err := json.Marshal(report.Items)
	   if err != nil {
		  return err
	  }
	  q := s.db.Conn(ctx)
	  _, err = q.ExecContext(ctx, ` 
INSERT INTO sr_residue_reports
  (id, saga_run_id, scan_epoch, items_json, total_amount, severity, scan_note, scanned_at)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE scan_note=VALUES(scan_note)`,
		   report.ID, report.SagaRunID, report.ScanEpoch, string(itemsJSON),
		   report.TotalAmount, string(report.Severity), report.ScanNote, report.ScannedAt,
	   )
	  return err
   }

   func (s *SQLStore) GetLatestReport(ctx context.Context, sagaRunID string) (*ResidueReport, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	  }
	   q := s.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `
SELECT id, saga_run_id, scan_epoch, items_json, total_amount, severity, scan_note, scanned_at
FROM sr_residue_reports WHERE saga_run_id=? ORDER BY scan_epoch DESC LIMIT 1`, sagaRunID)
	   return scanReport(row)
  }

   func (s *SQLStore) ListReports(ctx context.Context, sagaRunID string, limit int) ([]ResidueReport, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	  }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	   }
	   q := s.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx, `
SELECT id, saga_run_id, scan_epoch, items_json, total_amount, severity, scan_note, scanned_at
FROM sr_residue_reports WHERE saga_run_id=? ORDER BY scan_epoch DESC LIMIT ?`, sagaRunID, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []ResidueReport
	  for rows.Next() {
		   var r ResidueReport
		  var itemsJSON string
		  if err := rows.Scan(&r.ID, &r.SagaRunID, &r.ScanEpoch, &itemsJSON,
			   &r.TotalAmount, &r.Severity, &r.ScanNote, &r.ScannedAt); err != nil {
			   return nil, err
		  }
		   _ = json.Unmarshal([]byte(itemsJSON), &r.Items)
		  out = append(out, r)
	   }
	  return out, rows.Err()
   }

// ── CloseoutPlan ──────────────────────────────────────────────────────────────

  func (s *SQLStore) SavePlan(ctx context.Context, plan *CloseoutPlan) error {
	  if s == nil || s.db == nil {
		  return ErrStorageNil
	   }
	  q := s.db.Conn(ctx)
	  _, err := q.ExecContext(ctx, `
INSERT INTO sr_closeout_plans
  (id, residue_run_id, saga_run_id, idem_prefix, authorizer1, authorizer2,
   generation, total_legs, applied_legs, failed_legs, skipped_legs,
   state, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  applied_legs=VALUES(applied_legs), failed_legs=VALUES(failed_legs),
  skipped_legs=VALUES(skipped_legs), state=VALUES(state), updated_at=VALUES(updated_at)`,
		   plan.ID, plan.ResidueRunID, plan.SagaRunID, plan.IdemPrefix,
		  plan.Authorizer1,
		   plan.Authorizer2, plan.Generation,
		   plan.TotalLegs, plan.AppliedLegs, plan.FailedLegs, plan.SkippedLegs,
		  string(plan.State), plan.CreatedAt, plan.UpdatedAt,
	   )
	  return err
   }

   func (s *SQLStore) GetPlan(ctx context.Context, id string) (*CloseoutPlan, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStorageNil
	   }
	   q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, ` 
SELECT id, residue_run_id, saga_run_id, idem_prefix, authorizer1, authorizer2,
       generation, total_legs, applied_legs, failed_legs, skipped_legs,
       state, created_at, updated_at
FROM sr_closeout_plans WHERE id=?`, id)
	  return scanPlan(row)
   }

  func (s *SQLStore) GetActivePlan(ctx context.Context, residueRunID string) (*CloseoutPlan, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStorageNil
	  }
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT id, residue_run_id, saga_run_id, idem_prefix, authorizer1, authorizer2,
       generation, total_legs, applied_legs, failed_legs, skipped_legs,
       state, created_at, updated_at
FROM sr_closeout_plans
WHERE residue_run_id=? AND state IN ('closing','residue_open')
ORDER BY created_at DESC LIMIT 1`, residueRunID)
	  return scanPlan(row)
   }

// ── CloseoutLeg ───────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveLeg(ctx context.Context, leg *CloseoutLeg) error {
	   if s == nil || s.db == nil {
		   return ErrStorageNil
	   }
	  q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx, `
INSERT INTO sr_closeout_legs
  (id, plan_id, residue_run_id, seq_no, kind, step, idem_suffix, amount, asset,
   direction, state, ledger_entry_id, fail_reason, skip_reason, retry_count,
   applied_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), ledger_entry_id=VALUES(ledger_entry_id),
  fail_reason=VALUES(fail_reason), skip_reason=VALUES(skip_reason),
  retry_count=VALUES(retry_count), applied_at=VALUES(applied_at),
  updated_at=VALUES(updated_at)`,
		   leg.ID, leg.PlanID, leg.ResidueRunID, leg.SeqNo, string(leg.Kind), leg.Step,
		  leg.IdemSuffix, leg.Amount, leg.Asset, leg.Direction, string(leg.State),
		  nullableStr(leg.LedgerEntryID),
		   nullableStr(leg.FailReason), nullableStr(leg.SkipReason),
		  leg.RetryCount, nullableTime(leg.AppliedAt), leg.CreatedAt, leg.UpdatedAt,
	  )
	   return err
   }

   func (s *SQLStore) GetLeg(ctx context.Context, id string) (*CloseoutLeg, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	  }
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT id, plan_id, residue_run_id, seq_no, kind, step, idem_suffix, amount, asset,
       direction, state, ledger_entry_id, fail_reason, skip_reason, retry_count,
       applied_at, created_at, updated_at
FROM sr_closeout_legs WHERE id=?`, id)
	   return scanLeg(row)
   }

  func (s *SQLStore) ListLegs(ctx context.Context, req LegListReq) ([]CloseoutLeg, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	  }
	  limit := req.Limit
	   if limit <= 0 || limit > 10000 {
		   limit = 200
	   }
	  where := []string{"1=1"}
	  args := []any{}
	  if req.PlanID != "" {
		   where = append(where, "plan_id=?")
		   args = append(args, req.PlanID)
	  }
	   if req.ResidueRunID != "" {
		  where = append(where, "residue_run_id=?")
		   args = append(args, req.ResidueRunID)
	  }
	   if req.State != "" {
		   where = append(where, "state=?")
		  args = append(args, string(req.State))
	   }
	  args = append(args, limit)
	   q := s.db.Conn(ctx)
	   rows, err := q.QueryContext(ctx, fmt.Sprintf(`
SELECT id, plan_id, residue_run_id, seq_no, kind, step, idem_suffix, amount, asset,
       direction, state, ledger_entry_id, fail_reason, skip_reason, retry_count,
       applied_at, created_at, updated_at
FROM sr_closeout_legs WHERE %s ORDER BY seq_no ASC LIMIT ?`, strings.Join(where, " AND ")), args...)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   var out []CloseoutLeg
	   for rows.Next() {
		   leg, err := scanLegRow(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, *leg)
	  }
	   return out, rows.Err()
   }

// ── CloseProof ────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveProof(ctx context.Context, proof *CloseProof) error {
	   if s == nil || s.db == nil {
		  return ErrStorageNil
	  }
	   q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx, `
INSERT INTO sr_close_proofs
  (id, residue_run_id, saga_run_id, plan_id, intent_hash, residue_hash,
   leg_hash, proof_digest, is_valid, verified_at, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  is_valid=VALUES(is_valid), verified_at=VALUES(verified_at)`,
		  proof.ID,
		  proof.ResidueRunID, proof.SagaRunID, proof.PlanID,
		   proof.IntentHash, proof.ResidueHash, proof.LegHash, proof.ProofDigest,
		  proof.IsValid, proof.VerifiedAt, proof.CreatedAt,
	  )
	   return err
  }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (*CloseProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStorageNil
	  }
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT id, residue_run_id, saga_run_id, plan_id, intent_hash, residue_hash,
       leg_hash, proof_digest, is_valid, verified_at, created_at
FROM sr_close_proofs WHERE id=?`, id)
	  return scanProof(row)
   }

  func (s *SQLStore) GetProofByRunID(ctx context.Context, residueRunID string) (*CloseProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	   }
	   q := s.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `
SELECT id, residue_run_id, saga_run_id, plan_id, intent_hash, residue_hash,
       leg_hash, proof_digest, is_valid, verified_at, created_at
FROM sr_close_proofs WHERE residue_run_id=? ORDER BY created_at DESC LIMIT 1`, residueRunID)
	  return scanProof(row)
  }

// ── IdemBlock ─────────────────────────────────────────────────────────────────

  func (s *SQLStore) BlockIdem(ctx context.Context, idemKey, reason string) error {
	  if s == nil || s.db == nil {
		   return ErrStorageNil
	  }
	  q := s.db.Conn(ctx)
	  _, err := q.ExecContext(ctx, ` 
INSERT INTO sr_idem_blocks (idem_key, reason, blocked_at)
VALUES (?,?,?) ON DUPLICATE KEY UPDATE reason=reason`,
		   idemKey, reason, time.Now().UTC(),
	  )
	  return err
  }

   func (s *SQLStore) IsIdemBlocked(ctx context.Context, idemKey string) (bool, error) {
	   if s == nil || s.db == nil {
		  return false, ErrStorageNil
	   }
	   q := s.db.Conn(ctx)
	  var count int
	  err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sr_idem_blocks WHERE idem_key=?`, idemKey).Scan(&count)
	   if err != nil {
		   return false, err
	  }
	   return count > 0, nil
   }

// ── MutexStore 实现 ──────────────────────────────────────────────────────────
// 注意：SQLStore 同时实现 MutexStore 接口。
// 以下方法签名与 MutexStore 接口完全对应。

   func (s *SQLStore) AcquireMutex(ctx context.Context, sagaRunID, holderID string, ttl time.Duration) (*MutexGen, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStorageNil
	   }
	  now := time.Now().UTC()
	   expiresAt := now.Add(ttl)
	   var mg MutexGen
	  err := s.db.InTx(ctx, func(ctx context.Context) error {
		  q := s.db.Conn(ctx)
		   // 检查现有锁
		  row := q.QueryRowContext(ctx, `
SELECT id, saga_run_id, generation, holder, status, expires_at, locked_at, released_at, created_at, updated_at
FROM sr_mutex_gens WHERE saga_run_id=? FOR UPDATE`, sagaRunID)
		  existing := &MutexGen{}
		   var lockedAt, releasedAt sql.NullTime
		  scanErr := row.Scan(&existing.ID, &existing.SagaRunID, &existing.Generation,
			   &existing.Holder,
			  &existing.Status, &existing.ExpiresAt,
			  &lockedAt,
			   &releasedAt, &existing.CreatedAt, &existing.UpdatedAt)
		  if scanErr == sql.ErrNoRows {
			   // 首次写入
			   existing = &MutexGen{
				   ID: newID(),
				   SagaRunID:  sagaRunID,
				  Generation: 0,
				   Status:     "open",
				   CreatedAt:  now,
			  }
		   } else if scanErr != nil {
			   return scanErr
		   } else {
			  if lockedAt.Valid {
				  existing.LockedAt = &lockedAt.Time
			  }
			   if releasedAt.Valid {
				   existing.ReleasedAt = &releasedAt.Time
			   }
		  }
		   // 检查有效锁
		  if existing.Status == "locked" && now.Before(existing.ExpiresAt) {
			   return ErrGenLocked
		   }
		   // 升代获取
		  existing.Generation++
		  existing.Holder = holderID
		  existing.Status = "locked"
		  existing.ExpiresAt = expiresAt
		   existing.LockedAt = &now
		  existing.UpdatedAt = now
		   _, upsertErr := q.ExecContext(ctx, `
INSERT INTO sr_mutex_gens
  (id, saga_run_id, generation, holder, status, expires_at, locked_at, released_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,NULL,?,?)
ON DUPLICATE KEY UPDATE
  generation=VALUES(generation), holder=VALUES(holder), status=VALUES(status),
  expires_at=VALUES(expires_at), locked_at=VALUES(locked_at),
  released_at=NULL, updated_at=VALUES(updated_at)`,
			  existing.ID, sagaRunID, existing.Generation, holderID, "locked",
			  expiresAt, now, existing.CreatedAt, now,
		  )
		  if upsertErr != nil {
			   return upsertErr
		   }
		   mg = *existing
		  return nil
	  })
	   if err != nil {
		  return nil, err
	   }
	  return &mg, nil
   }

  func (s *SQLStore) RefreshMutex(ctx context.Context, sagaRunID string, generation int64, holderID string, ttl time.Duration) error {
	   if s == nil || s.db == nil {
		  return ErrStorageNil
	   }
	  newExpires := time.Now().UTC().Add(ttl)
	  q := s.db.Conn(ctx)
	   res, err := q.ExecContext(ctx, `
UPDATE sr_mutex_gens SET expires_at=?, updated_at=?
WHERE saga_run_id=? AND generation=? AND holder=? AND status='locked'`,
		   newExpires, time.Now().UTC(), sagaRunID, generation, holderID,
	  )
	  if err != nil {
		  return err
	   }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrStaleGen
	   }
	   return nil
   }

  func (s *SQLStore) ReleaseMutex(ctx context.Context,
	   sagaRunID string, generation int64, holderID string) error {
	  if s == nil || s.db == nil {
		   return ErrStorageNil
	  }
	   now := time.Now().UTC()
	  q := s.db.Conn(ctx)
	  res, err := q.ExecContext(ctx, `
UPDATE sr_mutex_gens SET status='released', released_at=?, updated_at=?
WHERE saga_run_id=? AND generation=? AND holder=?`,
		  now, now, sagaRunID, generation, holderID,
	  )
	  if err != nil {
		  return err
	   }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrStaleGen
	   }
	   return nil
   }

  func (s *SQLStore) GetMutex(ctx context.Context, sagaRunID string) (*MutexGen, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStorageNil
	   }
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `
SELECT id, saga_run_id, generation, holder, status, expires_at, locked_at, released_at, created_at, updated_at
FROM sr_mutex_gens WHERE saga_run_id=?`, sagaRunID)
	   var mg MutexGen
	  var lockedAt, releasedAt sql.NullTime
	   err := row.Scan(&mg.ID, &mg.SagaRunID, &mg.Generation, &mg.Holder, &mg.Status,
		   &mg.ExpiresAt,
		  &lockedAt, &releasedAt, &mg.CreatedAt, &mg.UpdatedAt)
	   if err == sql.ErrNoRows {
		   return nil, ErrSagaRunNotFound
	  }
	  if err != nil {
		  return nil, err
	  }
	  if lockedAt.Valid {
		  mg.LockedAt = &lockedAt.Time
	   }
	  if releasedAt.Valid {
		  mg.ReleasedAt = &releasedAt.Time
	  }
	  return &mg, nil
  }

   func (s *SQLStore) ReclaimExpiredMutexes(ctx context.Context) (int, error) {
	  if s == nil || s.db == nil {
		   return 0, ErrStorageNil
	  }
	  now := time.Now().UTC()
	   q := s.db.Conn(ctx)
	  res, err := q.ExecContext(ctx, `
UPDATE sr_mutex_gens SET status='expired', updated_at=?
WHERE status='locked' AND expires_at < ?`, now, now)
	   if err != nil {
		  return 0, err
	  }
	  n, _ := res.RowsAffected()
	   return int(n), nil
  }

// ── 扫描辅助函数 ──────────────────────────────────────────────────────────────

   func scanResidueRun(row *sql.Row) (*ResidueRun, error) {
	  var r ResidueRun
	  var stepsJSON string
	  var planID, proofID sql.NullString
	  var escalatedAt, closedAt sql.NullTime
	  err := row.Scan(&r.ID, &r.SagaRunID, &r.UserID, &r.Asset, &stepsJSON,
		  &r.PartialCause,
		   &r.State, &r.Severity, &r.MutexGeneration,
		   &planID, &proofID, &escalatedAt, &closedAt, &r.CreatedAt, &r.UpdatedAt)
	   if err == sql.ErrNoRows {
		  return nil, ErrResidueRunNotFound
	   }
	  if err != nil {
		  return nil, err
	   }
	  _ = json.Unmarshal([]byte(stepsJSON), &r.PartialDoneSteps)
	   if planID.Valid {
		   r.CloseoutPlanID = planID.String
	   }
	   if proofID.Valid {
		   r.CloseProofID = proofID.String
	   }
	   if escalatedAt.Valid {
		  r.EscalatedAt = &escalatedAt.Time
	   }
	  if closedAt.Valid {
		   r.ClosedAt = &closedAt.Time
	   }
	  return &r, nil
   }

  func scanResidueRunRow(rows *sql.Rows) (*ResidueRun, error) {
	   var r ResidueRun
	  var stepsJSON string
	   var planID, proofID sql.NullString
	   var escalatedAt, closedAt sql.NullTime
	   err := rows.Scan(&r.ID,
		  &r.SagaRunID, &r.UserID, &r.Asset, &stepsJSON,
		   &r.PartialCause, &r.State, &r.Severity, &r.MutexGeneration,
		  &planID, &proofID, &escalatedAt, &closedAt, &r.CreatedAt, &r.UpdatedAt)
	   if err != nil {
		   return nil, err
	   }
	  _ = json.Unmarshal([]byte(stepsJSON), &r.PartialDoneSteps)
	  if planID.Valid {
		  r.CloseoutPlanID = planID.String
	  }
	   if proofID.Valid {
		   r.CloseProofID = proofID.String
	  }
	   if escalatedAt.Valid {
		  r.EscalatedAt = &escalatedAt.Time
	  }
	  if closedAt.Valid {
		  r.ClosedAt = &closedAt.Time
	  }
	   return &r, nil
  }

   func scanReport(row *sql.Row) (*ResidueReport, error) {
	  var r ResidueReport
	  var itemsJSON string
	  err := row.Scan(&r.ID, &r.SagaRunID, &r.ScanEpoch, &itemsJSON,
		  &r.TotalAmount, &r.Severity, &r.ScanNote, &r.ScannedAt)
	  if err == sql.ErrNoRows {
		   return nil, ErrResidueRunNotFound
	   }
	   if err != nil {
		  return nil, err
	   }
	  _ = json.Unmarshal([]byte(itemsJSON), &r.Items)
	  return &r, nil
  }

  func scanPlan(row *sql.Row) (*CloseoutPlan, error) {
	  var p CloseoutPlan
	  var state string
	   err := row.Scan(&p.ID, &p.ResidueRunID, &p.SagaRunID, &p.IdemPrefix,
		  &p.Authorizer1, &p.Authorizer2, &p.Generation,
		   &p.TotalLegs, &p.AppliedLegs, &p.FailedLegs, &p.SkippedLegs,
		   &state, &p.CreatedAt, &p.UpdatedAt)
	   if err == sql.ErrNoRows {
		  return nil, ErrResidueRunNotFound
	  }
	   if err != nil {
		   return nil, err
	  }
	   p.State = RunState(state)
	   return &p, nil
   }

  func scanLeg(row *sql.Row) (*CloseoutLeg, error) {
	  var l CloseoutLeg
	   var ledgerID, failReason, skipReason sql.NullString
	  var appliedAt sql.NullTime
	  err := row.Scan(&l.ID, &l.PlanID, &l.ResidueRunID, &l.SeqNo, &l.Kind, &l.Step,
		  &l.IdemSuffix, &l.Amount, &l.Asset, &l.Direction, &l.State,
		   &ledgerID, &failReason, &skipReason, &l.RetryCount,
		  &appliedAt, &l.CreatedAt, &l.UpdatedAt)
	  if err == sql.ErrNoRows {
		  return nil, ErrLegNotFound
	   }
	   if err != nil {
		   return nil, err
	  }
	  l.LedgerEntryID = ledgerID.String
	  l.FailReason = failReason.String
	   l.SkipReason = skipReason.String
	  if appliedAt.Valid {
		   l.AppliedAt = &appliedAt.Time
	   }
	   return &l, nil
   }

   func scanLegRow(rows *sql.Rows) (*CloseoutLeg, error) {
	  var l CloseoutLeg
	  var ledgerID, failReason, skipReason sql.NullString
	   var appliedAt sql.NullTime
	  err := rows.Scan(&l.ID, &l.PlanID, &l.ResidueRunID, &l.SeqNo, &l.Kind, &l.Step,
		  &l.IdemSuffix, &l.Amount, &l.Asset, &l.Direction, &l.State,
		  &ledgerID,
		  &failReason, &skipReason, &l.RetryCount,
		  &appliedAt, &l.CreatedAt, &l.UpdatedAt)
	   if err != nil {
		   return nil, err
	   }
	  l.LedgerEntryID = ledgerID.String
	   l.FailReason = failReason.String
	  l.SkipReason = skipReason.String
	  if appliedAt.Valid {
		   l.AppliedAt = &appliedAt.Time
	  }
	  return &l, nil
  }

   func scanProof(row *sql.Row) (*CloseProof, error) {
	  var p CloseProof
	  err := row.Scan(&p.ID,
		  &p.ResidueRunID, &p.SagaRunID, &p.PlanID,
		   &p.IntentHash, &p.ResidueHash, &p.LegHash, &p.ProofDigest,
		  &p.IsValid, &p.VerifiedAt, &p.CreatedAt)
	  if err == sql.ErrNoRows {
		  return nil, ErrProofNotFound
	  }
	   if err != nil {
		   return nil, err
	   }
	   return &p, nil
  }

// ── nullable 辅助 ─────────────────────────────────────────────────────────────

   func nullableStr(s string) sql.NullString {
	   if s == "" {
		   return sql.NullString{}
	   }
	   return sql.NullString{String: s, Valid: true}
   }

   func nullableTime(t *time.Time) sql.NullTime {
	  if t == nil {
		   return sql.NullTime{}
	   }
	  return sql.NullTime{Time: *t, Valid: true}
   }
