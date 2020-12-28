package wrfreeze

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// SQLStore SQL 持久化实现。

type SQLStore struct {
	db *sql.DB
	dialect string // "mysql" or "postgres"
}

// NewSQLStore 构造 SQL 存储。dialect 为 "mysql" 或 "postgres"。
   func NewSQLStore(db *sql.DB, dialect string) *SQLStore {
	  return &SQLStore{db: db, dialect: dialect}
  }

// ── CascadePlan ──

   func (s *SQLStore) SavePlan(ctx context.Context, plan *CascadePlan) error {
	  userSetJSON, _ := json.Marshal(plan.UserSet)
	   var sealedAt *time.Time
	   if plan.SealedAt != nil {
		   t := *plan.SealedAt
		  sealedAt = &t
	   }
	  if s.dialect == "postgres" {
		  _, err := s.db.ExecContext(ctx, `
INSERT INTO cascade_plans
  (id, incident_ref, scope, asset, cohort_tag, user_set, status,
   opened_by, armed_by, aborted_by, abort_reason, dual_req_id,
   expires_at, sealed_at, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (id) DO UPDATE SET
  status=EXCLUDED.status, armed_by=EXCLUDED.armed_by,
  aborted_by=EXCLUDED.aborted_by, abort_reason=EXCLUDED.abort_reason,
  sealed_at=EXCLUDED.sealed_at, updated_at=EXCLUDED.updated_at`,
			   plan.ID, plan.IncidentRef, string(plan.Scope), plan.Asset, plan.CohortTag,
			   string(userSetJSON), string(plan.Status),
			  plan.OpenedBy, plan.ArmedBy, plan.AbortedBy, plan.AbortReason, plan.DualReqID,
			   plan.ExpiresAt, sealedAt, plan.CreatedAt, plan.UpdatedAt)
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO cascade_plans
  (id, incident_ref, scope, asset, cohort_tag, user_set, status,
   opened_by, armed_by, aborted_by, abort_reason, dual_req_id,
   expires_at, sealed_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  status=VALUES(status), armed_by=VALUES(armed_by),
  aborted_by=VALUES(aborted_by), abort_reason=VALUES(abort_reason),
  sealed_at=VALUES(sealed_at), updated_at=VALUES(updated_at)`,
		   plan.ID, plan.IncidentRef, string(plan.Scope), plan.Asset, plan.CohortTag,
		   string(userSetJSON), string(plan.Status),
		  plan.OpenedBy, plan.ArmedBy, plan.AbortedBy, plan.AbortReason, plan.DualReqID,
		   plan.ExpiresAt,
		  sealedAt, plan.CreatedAt, plan.UpdatedAt)
	   return err
   }

   func (s *SQLStore) GetPlan(ctx context.Context, planID string) (*CascadePlan, error) {
	  row := s.db.QueryRowContext(ctx, `
SELECT id, incident_ref, scope, asset, cohort_tag, user_set, status,
       opened_by, armed_by, aborted_by, abort_reason, dual_req_id,
       expires_at, sealed_at, created_at, updated_at
FROM cascade_plans WHERE id = `+s.ph(1), planID)
	  return scanPlan(row)
   }

   func (s *SQLStore) ListPlans(ctx context.Context, status string, limit int) ([]*CascadePlan, error) {
	   query := `SELECT id, incident_ref, scope, asset, cohort_tag, user_set, status,
       opened_by, armed_by, aborted_by, abort_reason, dual_req_id,
       expires_at, sealed_at, created_at, updated_at
FROM cascade_plans`
	  args := []any{}
	   if status != "" {
		   query += " WHERE status = " + s.ph(1)
		   args = append(args, status)
	  }
	   query += " ORDER BY created_at DESC"
	   if limit > 0 {
		  query += " LIMIT " + itoa(limit)
	   }
	   rows, err := s.db.QueryContext(ctx, query, args...)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  return scanPlans(rows)
   }

  func (s *SQLStore) ListOverduePlans(ctx context.Context, now time.Time) ([]*CascadePlan, error) {
	   rows, err := s.db.QueryContext(ctx, `
SELECT id, incident_ref, scope, asset, cohort_tag, user_set, status,
       opened_by, armed_by, aborted_by, abort_reason, dual_req_id,
       expires_at, sealed_at, created_at, updated_at
FROM cascade_plans
WHERE status NOT IN ('sealed','aborted') AND expires_at < `+s.ph(1), now)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  return scanPlans(rows)
  }

   func scanPlan(row *sql.Row) (*CascadePlan, error) {
	  var p CascadePlan
	   var userSetJSON string
	   var sealedAt sql.NullTime
	  var armedBy, abortedBy, abortReason, dualReqID, asset, cohortTag sql.NullString
	   err := row.Scan(
		   &p.ID, &p.IncidentRef, &p.Scope, &asset, &cohortTag, &userSetJSON, &p.Status,
		   &p.OpenedBy, &armedBy, &abortedBy, &abortReason, &dualReqID,
		   &p.ExpiresAt, &sealedAt, &p.CreatedAt, &p.UpdatedAt)
	  if err == sql.ErrNoRows {
		  return nil, ErrPlanNotFound
	  }
	  if err != nil {
		  return nil, err
	  }
	   p.Asset = asset.String
	  p.CohortTag = cohortTag.String
	  p.ArmedBy = armedBy.String
	   p.AbortedBy = abortedBy.String
	   p.AbortReason = abortReason.String
	  p.DualReqID = dualReqID.String
	  if sealedAt.Valid {
		  t := sealedAt.Time
		  p.SealedAt = &t
	   }
	   _ = json.Unmarshal([]byte(userSetJSON), &p.UserSet)
	  return &p, nil
   }

   func scanPlans(rows *sql.Rows) ([]*CascadePlan, error) {
	  out := make([]*CascadePlan, 0)
	  for rows.Next() {
		  var p CascadePlan
		  var userSetJSON string
		  var sealedAt sql.NullTime
		  var armedBy, abortedBy, abortReason, dualReqID, asset, cohortTag sql.NullString
		   if err := rows.Scan(
			  &p.ID, &p.IncidentRef, &p.Scope, &asset, &cohortTag, &userSetJSON, &p.Status,
			   &p.OpenedBy, &armedBy, &abortedBy, &abortReason, &dualReqID,
			  &p.ExpiresAt, &sealedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			  return nil, err
		   }
		  p.Asset = asset.String
		  p.CohortTag = cohortTag.String
		  p.ArmedBy = armedBy.String
		   p.AbortedBy = abortedBy.String
		  p.AbortReason = abortReason.String
		   p.DualReqID = dualReqID.String
		   if sealedAt.Valid {
			   t := sealedAt.Time
			  p.SealedAt = &t
		  }
		  _ = json.Unmarshal([]byte(userSetJSON), &p.UserSet)
		   out = append(out, &p)
	  }
	   return out, rows.Err()
   }

// ── FreezeWave ──

   func (s *SQLStore) SaveWave(ctx context.Context, wave *FreezeWave) error {
	  usersJSON, _ := json.Marshal(wave.TargetUsers)
	  if s.dialect == "postgres" {
		  _, err := s.db.ExecContext(ctx, `
INSERT INTO freeze_waves
  (id, plan_id, seq_no, label, target_users, status,
   frozen_count, failed_count, retry_count, started_at, finished_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (id) DO UPDATE SET
  status=EXCLUDED.status, frozen_count=EXCLUDED.frozen_count,
  failed_count=EXCLUDED.failed_count, retry_count=EXCLUDED.retry_count,
  started_at=EXCLUDED.started_at, finished_at=EXCLUDED.finished_at`,
			   wave.ID, wave.PlanID, wave.SeqNo, wave.Label, string(usersJSON), string(wave.Status),
			   wave.FrozenCount, wave.FailedCount, wave.RetryCount, wave.StartedAt, wave.FinishedAt, wave.CreatedAt)
		  return err
	   }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO freeze_waves
  (id, plan_id, seq_no, label, target_users, status,
   frozen_count, failed_count, retry_count, started_at, finished_at, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  status=VALUES(status), frozen_count=VALUES(frozen_count),
  failed_count=VALUES(failed_count), retry_count=VALUES(retry_count),
  started_at=VALUES(started_at), finished_at=VALUES(finished_at)`,
		  wave.ID,
		   wave.PlanID, wave.SeqNo, wave.Label, string(usersJSON), string(wave.Status),
		   wave.FrozenCount, wave.FailedCount, wave.RetryCount, wave.StartedAt, wave.FinishedAt, wave.CreatedAt)
	  return err
   }

   func (s *SQLStore) GetWave(ctx context.Context, waveID string) (*FreezeWave, error) {
	  row := s.db.QueryRowContext(ctx, ` 
SELECT id, plan_id, seq_no, label, target_users, status,
       frozen_count, failed_count, retry_count, started_at, finished_at, created_at
FROM freeze_waves WHERE id = `+s.ph(1), waveID)
	   return scanWave(row)
   }

  func (s *SQLStore) ListWaves(ctx context.Context, planID string) ([]*FreezeWave, error) {
	   rows, err := s.db.QueryContext(ctx, ` 
SELECT id, plan_id, seq_no, label, target_users, status,
       frozen_count, failed_count, retry_count, started_at, finished_at, created_at
FROM freeze_waves WHERE plan_id = `+s.ph(1)+` ORDER BY seq_no ASC`, planID)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanWaves(rows)
   }

  func (s *SQLStore) SaveWaveEntry(ctx context.Context, entry *WaveUserEntry) error {
	   if s.dialect == "postgres" {
		   _, err := s.db.ExecContext(ctx, `
INSERT INTO wave_user_entries (id, wave_id, plan_id, user_id, result, err_msg, attempt_n, at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (id) DO NOTHING`,
			   entry.ID, entry.WaveID, entry.PlanID, entry.UserID, string(entry.Result), entry.ErrMsg, entry.AttemptN, entry.At)
		  return err
	   }
	   _, err := s.db.ExecContext(ctx, `
INSERT IGNORE INTO wave_user_entries (id, wave_id, plan_id, user_id, result, err_msg, attempt_n, at)
VALUES (?,?,?,?,?,?,?,?)`,
		  entry.ID, entry.WaveID, entry.PlanID, entry.UserID, string(entry.Result), entry.ErrMsg, entry.AttemptN, entry.At)
	   return err
  }

  func scanWave(row *sql.Row) (*FreezeWave, error) {
	  var w FreezeWave
	  var usersJSON string
	   var startedAt, finishedAt sql.NullTime
	   var label sql.NullString
	   err := row.Scan(&w.ID, &w.PlanID, &w.SeqNo, &label, &usersJSON, &w.Status,
		   &w.FrozenCount, &w.FailedCount, &w.RetryCount, &startedAt, &finishedAt, &w.CreatedAt)
	  if err == sql.ErrNoRows {
		   return nil, ErrWaveNotFound
	   }
	  if err != nil {
		  return nil, err
	  }
	   w.Label = label.String
	   if startedAt.Valid {
		   t := startedAt.Time
		   w.StartedAt = &t
	   }
	   if finishedAt.Valid {
		   t := finishedAt.Time
		   w.FinishedAt = &t
	   }
	  _ = json.Unmarshal([]byte(usersJSON), &w.TargetUsers)
	  return &w, nil
   }

  func scanWaves(rows *sql.Rows) ([]*FreezeWave, error) {
	  out := make([]*FreezeWave, 0)
	   for rows.Next() {
		  var w FreezeWave
		  var usersJSON string
		   var startedAt, finishedAt sql.NullTime
		  var label sql.NullString
		   if err := rows.Scan(&w.ID, &w.PlanID, &w.SeqNo, &label, &usersJSON, &w.Status,
			  &w.FrozenCount, &w.FailedCount, &w.RetryCount, &startedAt, &finishedAt, &w.CreatedAt); err != nil {
			  return nil, err
		  }
		   w.Label = label.String
		   if startedAt.Valid {
			  t := startedAt.Time
			   w.StartedAt = &t
		   }
		   if finishedAt.Valid {
			  t := finishedAt.Time
			   w.FinishedAt = &t
		   }
		  _ = json.Unmarshal([]byte(usersJSON), &w.TargetUsers)
		   out = append(out, &w)
	   }
	  return out, rows.Err()
   }

// ── UnfreezePhase ──

   func (s *SQLStore) SavePhase(ctx context.Context, phase *UnfreezePhase) error {
	  if s.dialect == "postgres" {
		  _, err := s.db.ExecContext(ctx, `
INSERT INTO unfreeze_phases
  (id, plan_id, kind, status, dual_req_id, initiated_by, approved_by,
   integrity_note, unfrozen_count, failed_count, started_at, finished_at, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (id) DO UPDATE SET
  status=EXCLUDED.status, approved_by=EXCLUDED.approved_by,
  integrity_note=EXCLUDED.integrity_note, unfrozen_count=EXCLUDED.unfrozen_count,
  failed_count=EXCLUDED.failed_count, started_at=EXCLUDED.started_at,
  finished_at=EXCLUDED.finished_at, updated_at=EXCLUDED.updated_at`,
			   phase.ID, phase.PlanID, string(phase.Kind), string(phase.Status),
			   phase.DualReqID, phase.InitiatedBy, phase.ApprovedBy,
			  phase.IntegrityNote, phase.UnfrozenCount, phase.FailedCount,
			   phase.StartedAt,
			  phase.FinishedAt, phase.CreatedAt, phase.UpdatedAt)
		  return err
	   }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO unfreeze_phases
  (id, plan_id, kind, status, dual_req_id, initiated_by, approved_by,
   integrity_note, unfrozen_count, failed_count, started_at, finished_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  status=VALUES(status), approved_by=VALUES(approved_by),
  integrity_note=VALUES(integrity_note), unfrozen_count=VALUES(unfrozen_count),
  failed_count=VALUES(failed_count), started_at=VALUES(started_at),
  finished_at=VALUES(finished_at), updated_at=VALUES(updated_at)`,
		   phase.ID, phase.PlanID, string(phase.Kind), string(phase.Status),
		  phase.DualReqID, phase.InitiatedBy, phase.ApprovedBy,
		   phase.IntegrityNote, phase.UnfrozenCount, phase.FailedCount,
		  phase.StartedAt, phase.FinishedAt, phase.CreatedAt, phase.UpdatedAt)
	   return err
   }

   func (s *SQLStore) GetPhase(ctx context.Context, phaseID string) (*UnfreezePhase, error) {
	   row := s.db.QueryRowContext(ctx, ` 
SELECT id, plan_id, kind, status, dual_req_id, initiated_by, approved_by,
       integrity_note, unfrozen_count, failed_count, started_at, finished_at, created_at, updated_at
FROM unfreeze_phases WHERE id = `+s.ph(1), phaseID)
	   return scanPhase(row)
  }

   func (s *SQLStore) ListPhases(ctx context.Context, planID string) ([]*UnfreezePhase, error) {
	   rows, err := s.db.QueryContext(ctx, `
SELECT id, plan_id, kind, status, dual_req_id, initiated_by, approved_by,
       integrity_note, unfrozen_count, failed_count, started_at, finished_at, created_at, updated_at
FROM unfreeze_phases WHERE plan_id = `+s.ph(1)+` ORDER BY created_at ASC`, planID)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanPhases(rows)
   }

  func scanPhase(row *sql.Row) (*UnfreezePhase, error) {
	  var p UnfreezePhase
	  var startedAt, finishedAt sql.NullTime
	  var dualReqID, approvedBy, integrityNote sql.NullString
	  err := row.Scan(&p.ID, &p.PlanID, &p.Kind, &p.Status,
		   &dualReqID, &p.InitiatedBy, &approvedBy,
		  &integrityNote, &p.UnfrozenCount, &p.FailedCount,
		  &startedAt, &finishedAt, &p.CreatedAt, &p.UpdatedAt)
	  if err == sql.ErrNoRows {
		   return nil, ErrPhaseNotFound
	  }
	  if err != nil {
		  return nil, err
	   }
	  p.DualReqID = dualReqID.String
	  p.ApprovedBy = approvedBy.String
	   p.IntegrityNote = integrityNote.String
	  if startedAt.Valid {
		   t := startedAt.Time
		   p.StartedAt = &t
	   }
	   if finishedAt.Valid {
		   t := finishedAt.Time
		   p.FinishedAt = &t
	  }
	   return &p, nil
  }

  func scanPhases(rows *sql.Rows) ([]*UnfreezePhase, error) {
	  out := make([]*UnfreezePhase, 0)
	   for rows.Next() {
		  var p UnfreezePhase
		  var startedAt, finishedAt sql.NullTime
		  var dualReqID, approvedBy, integrityNote sql.NullString
		   if err := rows.Scan(&p.ID, &p.PlanID, &p.Kind, &p.Status,
			  &dualReqID, &p.InitiatedBy, &approvedBy,
			  &integrityNote,
			   &p.UnfrozenCount, &p.FailedCount,
			  &startedAt, &finishedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			  return nil, err
		   }
		   p.DualReqID = dualReqID.String
		   p.ApprovedBy = approvedBy.String
		  p.IntegrityNote = integrityNote.String
		   if startedAt.Valid {
			  t := startedAt.Time
			  p.StartedAt = &t
		  }
		   if finishedAt.Valid {
			  t := finishedAt.Time
			  p.FinishedAt = &t
		  }
		   out = append(out, &p)
	  }
	  return out, rows.Err()
  }

// ── CoverageProof ──

  func (s *SQLStore) SaveProof(ctx context.Context, proof *CoverageProof) error {
	  deltaJSON, _ := json.Marshal(proof.DeltaUsers)
	   if s.dialect == "postgres" {
		   _, err := s.db.ExecContext(ctx, `
INSERT INTO coverage_proofs
  (id, plan_id, expected_count, actual_count, delta_count, delta_users, verdict, checked_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO NOTHING`,
			  proof.ID, proof.PlanID, proof.ExpectedCount, proof.ActualCount,
			   proof.DeltaCount, string(deltaJSON), string(proof.Verdict), proof.CheckedAt, proof.CreatedAt)
		  return err
	   }
	  _, err := s.db.ExecContext(ctx, `
INSERT IGNORE INTO coverage_proofs
  (id, plan_id, expected_count, actual_count, delta_count, delta_users, verdict, checked_at, created_at)
VALUES (?,?,?,?,?,?,?,?,?)`,
		   proof.ID, proof.PlanID, proof.ExpectedCount, proof.ActualCount,
		  proof.DeltaCount,
		   string(deltaJSON), string(proof.Verdict), proof.CheckedAt, proof.CreatedAt)
	   return err
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*CoverageProof, error) {
	  row := s.db.QueryRowContext(ctx, `
SELECT id, plan_id, expected_count, actual_count, delta_count, delta_users, verdict, checked_at, created_at
FROM coverage_proofs WHERE id = `+s.ph(1), proofID)
	   return scanProof(row)
   }

   func (s *SQLStore) LatestProof(ctx context.Context, planID string) (*CoverageProof, error) {
	  row := s.db.QueryRowContext(ctx, `
SELECT id, plan_id, expected_count, actual_count, delta_count, delta_users, verdict, checked_at, created_at
FROM coverage_proofs WHERE plan_id = `+s.ph(1)+` ORDER BY checked_at DESC LIMIT 1`, planID)
	   p, err := scanProof(row)
	   if err != nil && strings.Contains(err.Error(), "not found") {
		  return nil, nil
	   }
	  return p, err
  }

  func (s *SQLStore) ListProofs(ctx context.Context, planID string) ([]*CoverageProof, error) {
	  rows, err := s.db.QueryContext(ctx, `
SELECT id, plan_id, expected_count, actual_count, delta_count, delta_users, verdict, checked_at, created_at
FROM coverage_proofs WHERE plan_id = `+s.ph(1)+` ORDER BY checked_at DESC`, planID)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]*CoverageProof, 0)
	   for rows.Next() {
		   var p CoverageProof
		  var deltaJSON string
		  if err := rows.Scan(&p.ID, &p.PlanID, &p.ExpectedCount, &p.ActualCount,
			   &p.DeltaCount, &deltaJSON, &p.Verdict, &p.CheckedAt, &p.CreatedAt); err != nil {
			  return nil, err
		   }
		   _ = json.Unmarshal([]byte(deltaJSON), &p.DeltaUsers)
		  out = append(out, &p)
	   }
	  return out, rows.Err()
  }

  func scanProof(row *sql.Row) (*CoverageProof, error) {
	   var p CoverageProof
	  var deltaJSON string
	   err := row.Scan(&p.ID, &p.PlanID, &p.ExpectedCount, &p.ActualCount,
		   &p.DeltaCount, &deltaJSON, &p.Verdict, &p.CheckedAt, &p.CreatedAt)
	  if err == sql.ErrNoRows {
		   return nil, ErrPlanNotFound
	   }
	  if err != nil {
		  return nil, err
	   }
	  _ = json.Unmarshal([]byte(deltaJSON), &p.DeltaUsers)
	  return &p, nil
   }

// ── RollbackLeg ──

   func (s *SQLStore) SaveLeg(ctx context.Context, leg *RollbackLeg) error {
	   if s.dialect == "postgres" {
		  _, err := s.db.ExecContext(ctx, `
INSERT INTO rollback_legs
  (id, plan_id, user_id, reason, status, dual_req_id, requested_by, executed_by,
   availability_note, has_live_hash, frozen_by_plan, amount, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (id) DO UPDATE SET
  status=EXCLUDED.status, executed_by=EXCLUDED.executed_by,
  availability_note=EXCLUDED.availability_note, updated_at=EXCLUDED.updated_at`,
			   leg.ID, leg.PlanID, leg.UserID, string(leg.Reason), string(leg.Status),
			   leg.DualReqID, leg.RequestedBy, leg.ExecutedBy,
			   leg.AvailabilityNote,
			  leg.HasLiveHash, leg.FrozenByPlan,
			   leg.Amount.String(), leg.CreatedAt, leg.UpdatedAt)
		  return err
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO rollback_legs
  (id, plan_id, user_id, reason, status, dual_req_id, requested_by, executed_by,
   availability_note, has_live_hash, frozen_by_plan, amount, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  status=VALUES(status), executed_by=VALUES(executed_by),
  availability_note=VALUES(availability_note), updated_at=VALUES(updated_at)`,
		  leg.ID,
		  leg.PlanID, leg.UserID, string(leg.Reason), string(leg.Status),
		  leg.DualReqID, leg.RequestedBy, leg.ExecutedBy,
		   leg.AvailabilityNote, leg.HasLiveHash, leg.FrozenByPlan,
		  leg.Amount.String(),
		   leg.CreatedAt, leg.UpdatedAt)
	   return err
   }

   func (s *SQLStore) GetLeg(ctx context.Context, legID string) (*RollbackLeg, error) {
	  row := s.db.QueryRowContext(ctx, `
SELECT id, plan_id, user_id, reason, status, dual_req_id, requested_by, executed_by,
       availability_note, has_live_hash, frozen_by_plan, amount, created_at, updated_at
FROM rollback_legs WHERE id = `+s.ph(1), legID)
	  return scanLeg(row)
   }

   func (s *SQLStore) ListLegs(ctx context.Context, planID string) ([]*RollbackLeg, error) {
	  rows, err := s.db.QueryContext(ctx, `
SELECT id, plan_id, user_id, reason, status, dual_req_id, requested_by, executed_by,
       availability_note, has_live_hash, frozen_by_plan, amount, created_at, updated_at
FROM rollback_legs WHERE plan_id = `+s.ph(1)+` ORDER BY created_at DESC`, planID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]*RollbackLeg, 0)
	  for rows.Next() {
		   var leg RollbackLeg
		  var executedBy,
			  dualReqID, availNote, amtStr sql.NullString
		  if err := rows.Scan(&leg.ID, &leg.PlanID, &leg.UserID, &leg.Reason, &leg.Status,
			   &dualReqID,
			  &leg.RequestedBy, &executedBy,
			   &availNote, &leg.HasLiveHash, &leg.FrozenByPlan, &amtStr,
			  &leg.CreatedAt, &leg.UpdatedAt); err != nil {
			  return nil, err
		   }
		  leg.ExecutedBy = executedBy.String
		   leg.DualReqID = dualReqID.String
		  leg.AvailabilityNote = availNote.String
		   if amtStr.Valid {
			  _ = leg.Amount.UnmarshalText([]byte(amtStr.String))
		  }
		   out = append(out, &leg)
	  }
	   return out, rows.Err()
  }

   func scanLeg(row *sql.Row) (*RollbackLeg, error) {
	   var leg RollbackLeg
	  var executedBy, dualReqID, availNote, amtStr sql.NullString
	  err := row.Scan(&leg.ID, &leg.PlanID, &leg.UserID, &leg.Reason, &leg.Status,
		   &dualReqID, &leg.RequestedBy, &executedBy,
		  &availNote, &leg.HasLiveHash, &leg.FrozenByPlan, &amtStr,
		  &leg.CreatedAt, &leg.UpdatedAt)
	  if err == sql.ErrNoRows {
		  return nil, ErrLegNotFound
	   }
	   if err != nil {
		   return nil, err
	   }
	   leg.ExecutedBy = executedBy.String
	   leg.DualReqID = dualReqID.String
	  leg.AvailabilityNote = availNote.String
	   if amtStr.Valid {
		   _ = leg.Amount.UnmarshalText([]byte(amtStr.String))
	   }
	   return &leg, nil
   }

// ph 生成参数占位符（MySQL: ? / Postgres: $N）。
  func (s *SQLStore) ph(n int) string {
	   if s.dialect == "postgres" {
		  return "$" + itoa(n)
	  }
	  return "?"
  }
