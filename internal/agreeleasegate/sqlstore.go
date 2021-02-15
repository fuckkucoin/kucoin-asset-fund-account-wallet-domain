// SQL Store 骨架：映射 alg_agree_* 表。由 persist.SQLDB 注入。
// 本实现提供语句与扫描约定，便于运维对账；单元默认走 MemStore。
// 不回写 sweeppol / treasury / coolmark / trexec 表。

// Author: Ronan
// Date: 2021.01.25

package agreeleasegate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLDB 最小执行面。

type SQLDB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context,
		query string, args ...any) *sql.Row
}

// SQLStore 关系库实现。

type SQLStore struct {
	db SQLDB
}

// NewSQLStore 构造。

  func NewSQLStore(db SQLDB) *SQLStore {
	   return &SQLStore{db: db}
  }

const qInsertLease = `INSERT INTO alg_agree_leases (
  id, chain, asset, agree_code, bal_fingerprint, rule_fingerprint, kinds, rule_version,
  shadow_bal_at_issue, waterline_at_issue, min_sweep_at_issue, state, idem_key, plan_ref, exec_ref,
  issued_at, expires_at, planned_at, executing_at, consumed_at, revoked_at, revoke_reason, drift_proof_id,
  created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

   func (s *SQLStore) InsertLease(ctx context.Context, l *AgreeLease) error {
	   if s == nil || s.db == nil {
		  return ErrGateUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, qInsertLease,
		   l.ID, l.Chain, l.Asset, l.AgreeCode, l.BalFingerprint, l.RuleFingerprint, l.Kinds, l.RuleVersion,
		   l.ShadowBalAtIssue,
		   l.WaterlineAtIssue, l.MinSweepAtIssue, string(l.State), l.IdemKey, l.PlanRef, l.ExecRef,
		   l.IssuedAt, l.ExpiresAt, nullTime(l.PlannedAt), nullTime(l.ExecutingAt), nullTime(l.ConsumedAt), nullTime(l.RevokedAt),
		  l.RevokeReason, l.DriftProofID, l.CreatedAt, l.UpdatedAt,
	  )
	   if err != nil {
		  return fmt.Errorf("agreeleasegate sql insert lease: %w", err)
	   }
	   return nil
  }

const qGetLease = `SELECT id, chain, asset, agree_code, bal_fingerprint, rule_fingerprint, kinds, rule_version,
  shadow_bal_at_issue, waterline_at_issue, min_sweep_at_issue, state, idem_key, plan_ref, exec_ref,
  issued_at, expires_at, planned_at, executing_at, consumed_at, revoked_at, revoke_reason, drift_proof_id,
  created_at, updated_at FROM alg_agree_leases WHERE id=?`

   func (s *SQLStore) GetLeaseByID(ctx context.Context, id string) (*AgreeLease, error) {
	   return s.scanLease(s.db.QueryRowContext(ctx, qGetLease, id))
  }

   func (s *SQLStore) GetLeaseByIdem(ctx context.Context, idemKey string) (*AgreeLease, error) {
	   const q = `SELECT id, chain, asset, agree_code, bal_fingerprint, rule_fingerprint, kinds, rule_version,
  shadow_bal_at_issue, waterline_at_issue, min_sweep_at_issue, state, idem_key, plan_ref, exec_ref,
  issued_at, expires_at, planned_at, executing_at, consumed_at, revoked_at, revoke_reason, drift_proof_id,
  created_at, updated_at FROM alg_agree_leases WHERE idem_key=?`
	  return s.scanLease(s.db.QueryRowContext(ctx, q, idemKey))
   }

   func (s *SQLStore) scanLease(row *sql.Row) (*AgreeLease, error) {
	   var l AgreeLease
	   var state string
	   var planned,
		  execAt, cons, rev sql.NullTime
	   err := row.Scan(
		   &l.ID, &l.Chain, &l.Asset, &l.AgreeCode, &l.BalFingerprint, &l.RuleFingerprint, &l.Kinds, &l.RuleVersion,
		   &l.ShadowBalAtIssue, &l.WaterlineAtIssue, &l.MinSweepAtIssue, &state, &l.IdemKey, &l.PlanRef, &l.ExecRef,
		  &l.IssuedAt, &l.ExpiresAt, &planned, &execAt, &cons, &rev, &l.RevokeReason, &l.DriftProofID,
		  &l.CreatedAt, &l.UpdatedAt,
	  )
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrNotFound
		   }
		  return nil, err
	  }
	  l.State = LeaseState(state)
	  l.PlannedAt = nullToPtr(planned)
	   l.ExecutingAt = nullToPtr(execAt)
	   l.ConsumedAt = nullToPtr(cons)
	  l.RevokedAt = nullToPtr(rev)
	   return &l, nil
  }

const qUpdateLease = `UPDATE alg_agree_leases SET state=?, plan_ref=?, exec_ref=?, planned_at=?, executing_at=?,
  consumed_at=?, revoked_at=?, revoke_reason=?, drift_proof_id=?, updated_at=? WHERE id=?`

   func (s *SQLStore) UpdateLease(ctx context.Context, l *AgreeLease) error {
	  _, err := s.db.ExecContext(ctx, qUpdateLease,
		  string(l.State), l.PlanRef, l.ExecRef, nullTime(l.PlannedAt), nullTime(l.ExecutingAt),
		  nullTime(l.ConsumedAt), nullTime(l.RevokedAt), l.RevokeReason, l.DriftProofID, l.UpdatedAt, l.ID,
	  )
	  return err
   }

   func (s *SQLStore) listLeases(ctx context.Context, q string, args ...any) ([]*AgreeLease, error) {
	   rows,
		   err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   var out []*AgreeLease
	  for rows.Next() {
		   var l AgreeLease
		  var state string
		   var planned, execAt, cons, rev sql.NullTime
		   if err := rows.Scan(
			   &l.ID, &l.Chain, &l.Asset, &l.AgreeCode, &l.BalFingerprint, &l.RuleFingerprint, &l.Kinds, &l.RuleVersion,
			  &l.ShadowBalAtIssue, &l.WaterlineAtIssue, &l.MinSweepAtIssue, &state, &l.IdemKey, &l.PlanRef, &l.ExecRef,
			  &l.IssuedAt,
  created_at, updated_at`

   func (s *SQLStore) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*AgreeLease, error) {
	   q := `SELECT ` + leaseCols + ` FROM alg_agree_leases WHERE state IN ('agreed','planned','executing') AND expires_at<=? ORDER BY expires_at ASC LIMIT ?`
	  return s.listLeases(ctx, q, before, limit)
   }

  func (s *SQLStore) ListActiveLeases(ctx context.Context, limit int) ([]*AgreeLease, error) {
	  q := `SELECT ` + leaseCols + ` FROM alg_agree_leases WHERE state IN ('agreed','planned','executing') ORDER BY issued_at ASC LIMIT ?`
	  return s.listLeases(ctx, q, limit)
  }

  func (s *SQLStore) ListActiveByAsset(ctx context.Context, asset string, limit int) ([]*AgreeLease, error) {
	  q := `SELECT ` + leaseCols + ` FROM alg_agree_leases WHERE state IN ('agreed','planned','executing') AND asset=? ORDER BY issued_at ASC LIMIT ?`
	  return s.listLeases(ctx, q, asset, limit)
  }

   func (s *SQLStore) ListPlannedStale(ctx context.Context, before time.Time, limit int) ([]*AgreeLease, error) {
	   q := `SELECT ` + leaseCols + ` FROM alg_agree_leases WHERE state='planned' AND planned_at<=? ORDER BY planned_at ASC LIMIT ?`
	   return s.listLeases(ctx, q, before, limit)
  }

const qInsertProof = `INSERT INTO alg_agree_drift_proofs (
  id, lease_id, chain, asset, drift_type, old_bal_fingerprint, new_bal_fingerprint,
  old_rule_fingerprint, new_rule_fingerprint, old_rule_version, new_rule_version,
  old_shadow_bal, new_shadow_bal, old_waterline, new_waterline, state, proof_hash, idem_key,
  opened_at, proven_at, closed_at, closed_by, notes, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

   func (s *SQLStore) InsertProof(ctx context.Context, p *AgreeDriftProof) error {
	  _, err := s.db.ExecContext(ctx, qInsertProof,
		   p.ID, p.LeaseID, p.Chain, p.Asset, string(p.DriftType), p.OldBalFingerprint, p.NewBalFingerprint,
		  p.OldRuleFingerprint, p.NewRuleFingerprint, p.OldRuleVersion, p.NewRuleVersion,
		  p.OldShadowBal,
		   p.NewShadowBal, p.OldWaterline, p.NewWaterline, string(p.State), p.ProofHash, p.IdemKey,
		   p.OpenedAt, nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.ClosedBy, p.Notes, p.CreatedAt, p.UpdatedAt,
	  )
	   return err
   }

  func (s *SQLStore) GetProofByID(ctx context.Context, id string) (*AgreeDriftProof, error) {
	   const q = `SELECT id, lease_id, chain, asset, drift_type, old_bal_fingerprint, new_bal_fingerprint,
  old_rule_fingerprint, new_rule_fingerprint, old_rule_version, new_rule_version,
  old_shadow_bal, new_shadow_bal, old_waterline, new_waterline, state, proof_hash, idem_key,
  opened_at, proven_at, closed_at, closed_by, notes, created_at, updated_at FROM alg_agree_drift_proofs WHERE id=?`
	  return scanProof(s.db.QueryRowContext(ctx, q, id))
   }

   func (s *SQLStore) GetProofByIdem(ctx context.Context, idemKey string) (*AgreeDriftProof, error) {
	  const q = `SELECT id, lease_id, chain, asset, drift_type, old_bal_fingerprint, new_bal_fingerprint, 
  old_rule_fingerprint, new_rule_fingerprint, old_rule_version, new_rule_version,
  old_shadow_bal, new_shadow_bal, old_waterline, new_waterline, state, proof_hash, idem_key,
  opened_at, proven_at, closed_at, closed_by, notes, created_at, updated_at FROM alg_agree_drift_proofs WHERE idem_key=?`
	   return scanProof(s.db.QueryRowContext(ctx, q, idemKey))
   }

   func (s *SQLStore) GetOpenProofByLease(ctx context.Context, leaseID string) (*AgreeDriftProof, error) {
	   const q = `SELECT id, lease_id, chain, asset, drift_type, old_bal_fingerprint, new_bal_fingerprint,
  old_rule_fingerprint, new_rule_fingerprint, old_rule_version, new_rule_version,
  old_shadow_bal, new_shadow_bal, old_waterline, new_waterline, state, proof_hash, idem_key,
  opened_at, proven_at, closed_at, closed_by, notes, created_at, updated_at
  FROM alg_agree_drift_proofs WHERE lease_id=? AND state='open' LIMIT 1`
	  return scanProof(s.db.QueryRowContext(ctx, q, leaseID))
   }

   func scanProof(row *sql.Row) (*AgreeDriftProof, error) {
	  var p AgreeDriftProof
	   var dt, st string
	  var proven, closed sql.NullTime
	  err := row.Scan(
		   &p.ID, &p.LeaseID, &p.Chain, &p.Asset, &dt, &p.OldBalFingerprint, &p.NewBalFingerprint,
		  &p.OldRuleFingerprint, &p.NewRuleFingerprint, &p.OldRuleVersion, &p.NewRuleVersion,
		  &p.OldShadowBal,
		  &p.NewShadowBal, &p.OldWaterline, &p.NewWaterline, &st, &p.ProofHash, &p.IdemKey,
		  &p.OpenedAt, &proven, &closed, &p.ClosedBy, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
	   )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrNotFound
		   }
		   return nil, err
	  }
	  p.DriftType = DriftType(dt)
	  p.State = ProofState(st)
	   p.ProvenAt = nullToPtr(proven)
	   p.ClosedAt = nullToPtr(closed)
	  return &p, nil
  }

   func (s *SQLStore) UpdateProof(ctx context.Context, p *AgreeDriftProof) error {
	   const q = `UPDATE alg_agree_drift_proofs SET state=?, proven_at=?, closed_at=?, closed_by=?, notes=?, updated_at=? WHERE id=?`
	   _, err := s.db.ExecContext(ctx, q, string(p.State), nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.ClosedBy, p.Notes, p.UpdatedAt, p.ID)
	   return err
   }

   func (s *SQLStore) ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*AgreeDriftProof, error) {
	   const q = `SELECT id, lease_id, chain, asset, drift_type, old_bal_fingerprint, new_bal_fingerprint,
  old_rule_fingerprint, new_rule_fingerprint, old_rule_version, new_rule_version,
  old_shadow_bal, new_shadow_bal, old_waterline, new_waterline, state, proof_hash, idem_key,
  opened_at, proven_at, closed_at, closed_by, notes, created_at, updated_at
  FROM alg_agree_drift_proofs WHERE state=? ORDER BY opened_at ASC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, string(state), limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   var out []*AgreeDriftProof
	  for rows.Next() {
		   var p AgreeDriftProof
		   var dt, st string
		   var proven, closed sql.NullTime
		  if err := rows.Scan(
			  &p.ID, &p.LeaseID, &p.Chain, &p.Asset, &dt, &p.OldBalFingerprint, &p.NewBalFingerprint,
			   &p.OldRuleFingerprint,
			  &p.NewRuleFingerprint, &p.OldRuleVersion, &p.NewRuleVersion,
			  &p.OldShadowBal, &p.NewShadowBal, &p.OldWaterline, &p.NewWaterline, &st, &p.ProofHash, &p.IdemKey,
			   &p.OpenedAt, &proven, &closed, &p.ClosedBy, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
		  ); err != nil {
			   return nil, err
		  }
		  p.DriftType = DriftType(dt)
		  p.State = ProofState(st)
		   p.ProvenAt = nullToPtr(proven)
		   p.ClosedAt = nullToPtr(closed)
		  out = append(out, &p)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) InsertAudit(ctx context.Context, a *LeaseAudit) error {
	   const q = `INSERT INTO alg_agree_lease_audit (id, lease_id, from_state, to_state, actor, note, created_at) VALUES (?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q, a.ID, a.LeaseID, a.FromState, a.ToState, a.Actor, a.Note, a.CreatedAt)
	   return err
  }

  func (s *SQLStore) ListAuditByLease(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error) {
	  const q = `SELECT id, lease_id, from_state, to_state, actor, note, created_at FROM alg_agree_lease_audit WHERE lease_id=? ORDER BY created_at ASC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, leaseID, limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []*LeaseAudit
	   for rows.Next() {
		  var a LeaseAudit
		   if err := rows.Scan(&a.ID, &a.LeaseID, &a.FromState, &a.ToState, &a.Actor, &a.Note, &a.CreatedAt); err != nil {
			   return nil, err
		  }
		  out = append(out, &a)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) InsertGeneration(ctx context.Context, g *AgreeGeneration) error {
	   const q = `INSERT INTO alg_agree_generations (id, scope, chain, asset, generation, rule_fingerprint, reason, created_at) VALUES (?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q, g.ID, g.Scope, g.Chain, g.Asset, g.Generation, g.RuleFingerprint, g.Reason, g.CreatedAt)
	  return err
  }

  func (s *SQLStore) ListGenerations(ctx context.Context,
	  limit int) ([]*AgreeGeneration, error) {
	  const q = `SELECT id, scope, chain, asset, generation, rule_fingerprint, reason, created_at FROM alg_agree_generations ORDER BY created_at ASC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  var out []*AgreeGeneration
	  for rows.Next() {
		  var g AgreeGeneration
		   if err := rows.Scan(&g.ID, &g.Scope, &g.Chain, &g.Asset, &g.Generation, &g.RuleFingerprint, &g.Reason, &g.CreatedAt); err != nil {
			  return nil, err
		  }
		  out = append(out, &g)
	  }
	   return out, rows.Err()
   }

  func (s *SQLStore) InsertHalfPlan(ctx context.Context, h *HalfPlanBlock) error {
	  blocked := 0
	   if h.Blocked {
		  blocked = 1
	  }
	  const q = `INSERT INTO alg_agree_half_plans (id, lease_id, plan_ref, asset, reason, blocked, created_at) VALUES (?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q, h.ID, h.LeaseID, h.PlanRef, h.Asset, h.Reason, blocked, h.CreatedAt)
	  return err
   }

  func (s *SQLStore) GetHalfPlanByLease(ctx context.Context, leaseID string) (*HalfPlanBlock, error) {
	   const q = `SELECT id, lease_id, plan_ref, asset, reason, blocked, created_at FROM alg_agree_half_plans WHERE lease_id=? LIMIT 1`
	   var h HalfPlanBlock
	   var blocked int
	  err := s.db.QueryRowContext(ctx, q, leaseID).Scan(&h.ID, &h.LeaseID, &h.PlanRef, &h.Asset, &h.Reason, &blocked, &h.CreatedAt)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrNotFound
		   }
		   return nil, err
	  }
	  h.Blocked = blocked == 1
	  return &h, nil
  }

   func (s *SQLStore) ListHalfPlans(ctx context.Context, limit int) ([]*HalfPlanBlock, error) {
	  const q = `SELECT id, lease_id, plan_ref, asset, reason, blocked, created_at FROM alg_agree_half_plans ORDER BY created_at ASC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []*HalfPlanBlock
	  for rows.Next() {
		  var h HalfPlanBlock
		  var blocked int
		  if err := rows.Scan(&h.ID, &h.LeaseID, &h.PlanRef, &h.Asset, &h.Reason, &blocked, &h.CreatedAt); err != nil {
			   return nil, err
		   }
		   h.Blocked = blocked == 1
		  out = append(out, &h)
	  }
	   return out, rows.Err()
  }

  func nullTime(t *time.Time) any {
	   if t == nil {
		  return nil
	   }
	  return *t
  }

   func nullToPtr(n sql.NullTime) *time.Time {
	   if !n.Valid {
		   return nil
	  }
	  t := n.Time
	  return &t
  }

// Statements 导出建表后运维核验用的语句清单。
   func Statements() []string {
	  return []string{
		  qInsertLease, qGetLease, qUpdateLease, qInsertProof,
	   }
  }
