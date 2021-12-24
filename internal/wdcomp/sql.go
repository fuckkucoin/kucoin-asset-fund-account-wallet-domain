package wdcomp

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wdrollback"
)

type mirror struct {
	db *persist.SQLDB
}

   func (m *mirror) on() bool {
	   return m != nil && m.db != nil && m.db.DB != nil
  }

  func (m *mirror) exec(ctx context.Context, q string, pg string, args ...any) error {
	  if !m.on() {
		   return nil
	  }
	  if m.db.Dialect == persist.Postgres && pg != "" {
		  q = pg
	   }
	   _,
		  err := m.db.Conn(ctx).ExecContext(ctx, q, args...)
	   return err
  }

  func b01(v bool) int {
	   if v {
		  return 1
	  }
	   return 0
  }

  func (o *Orchestrator) noteMirror(err error) {
	   if err != nil {
		   o.mirrorErr = err.Error()
	   }
   }

  func (o *Orchestrator) mirrorFailure(ctx context.Context, f *WdFailure) {
	   if o == nil || f == nil || o.sql == nil {
		  return
	  }
	   o.noteMirror(o.sql.upsertFailure(ctx, *f))
  }

   func (o *Orchestrator) mirrorPlan(ctx context.Context, p *CompensatePlan) {
	  if o == nil || p == nil || o.sql == nil {
		  return
	   }
	   o.noteMirror(o.sql.upsertPlan(ctx, *p))
   }

   func (o *Orchestrator) mirrorLeg(ctx context.Context, leg *CompensateLeg) {
	   if o == nil || leg == nil || o.sql == nil {
		  return
	  }
	   o.noteMirror(o.sql.upsertLeg(ctx, *leg))
  }

  func (o *Orchestrator) mirrorLegs(ctx context.Context, planID string) {
	  for _, leg := range o.legs[planID] {
		  o.mirrorLeg(ctx, leg)
	  }
  }

   func (o *Orchestrator) mirrorEntry(ctx context.Context, e wdrollback.Entry) {
	  if o == nil || o.sql == nil {
		   return
	   }
	   o.noteMirror(o.sql.upsertEntry(ctx, e))
   }

  func (o *Orchestrator) mirrorPolicy(ctx context.Context, p AutoPolicy) {
	   if o == nil || o.sql == nil {
		  return
	   }
	  o.noteMirror(o.sql.upsertPolicy(ctx, p))
  }

  func (m *mirror) upsertFailure(ctx context.Context, f WdFailure) error {
	   q := `INSERT INTO wd_failures (
id, idempotency_key, fingerprint, transfer_id, user_id, asset, chain, source, code, reason_code, reason, user_text,
tx_hash, live_hash, chain_blocked, broadcast_status, freeze_state, chain_state, shard_plan_id, shard_id, shard_seq,
amount, filled_amount, failed_amount, in_flight_amount, fee_reserved, occupancy, protected_amount, compensable,
available_snapshot, frozen_snapshot, fee_snapshot, status, plan_id, may_auto, occurred_at, ingested_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE status=VALUES(status), plan_id=VALUES(plan_id), updated_at=VALUES(updated_at), may_auto=VALUES(may_auto)`
	  pg := `INSERT INTO wd_failures (
id, idempotency_key, fingerprint, transfer_id, user_id, asset, chain, source, code, reason_code, reason, user_text,
tx_hash, live_hash, chain_blocked, broadcast_status, freeze_state, chain_state, shard_plan_id, shard_id, shard_seq,
amount, filled_amount, failed_amount, in_flight_amount, fee_reserved, occupancy, protected_amount, compensable,
available_snapshot, frozen_snapshot, fee_snapshot, status, plan_id, may_auto, occurred_at, ingested_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, plan_id=EXCLUDED.plan_id, updated_at=EXCLUDED.updated_at, may_auto=EXCLUDED.may_auto`
	   return m.exec(ctx, q, pg,
		  f.ID,
		  f.IdempotencyKey, f.Fingerprint, f.TransferID, f.UserID, f.Asset, f.Chain, f.Source, string(f.Code),
		   f.ReasonCode, f.Reason, f.UserText, f.TxHash, b01(f.LiveHash), b01(f.ChainBlocked), f.BroadcastStatus,
		   f.FreezeState,
		   f.ChainState, f.ShardPlanID, f.ShardID, f.ShardSeq, f.Amount, f.FilledAmount, f.FailedAmount,
		  f.InFlightAmount, f.FeeReserved, f.Occupancy, f.ProtectedAmount, f.Compensable, f.AvailableSnapshot,
		   f.FrozenSnapshot, f.FeeSnapshot, string(f.Status), f.PlanID, b01(f.MayAuto), f.OccurredAt, f.IngestedAt, f.UpdatedAt,
	  )
   }

  func (m *mirror) upsertPlan(ctx context.Context, p CompensatePlan) error {
	   q := `INSERT INTO compensate_plans (
id, failure_id, transfer_id, user_id, asset, state, auto_ok, force_human, live_hash, chain_blocked,
policy_code, policy_version, idempotency_key, human_ticket_id, human_actor, human_approver, human_reason,
abort_reason, timeout_at, created_at, updated_at, committed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE state=VALUES(state), human_ticket_id=VALUES(human_ticket_id), human_actor=VALUES(human_actor),
human_approver=VALUES(human_approver), human_reason=VALUES(human_reason), abort_reason=VALUES(abort_reason),
updated_at=VALUES(updated_at), committed_at=VALUES(committed_at), auto_ok=VALUES(auto_ok), force_human=VALUES(force_human)`
	   pg := `INSERT INTO compensate_plans (
id, failure_id, transfer_id, user_id, asset, state, auto_ok, force_human, live_hash, chain_blocked,
policy_code, policy_version, idempotency_key, human_ticket_id, human_actor, human_approver, human_reason,
abort_reason, timeout_at, created_at, updated_at, committed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state, human_ticket_id=EXCLUDED.human_ticket_id, human_actor=EXCLUDED.human_actor,
human_approver=EXCLUDED.human_approver, human_reason=EXCLUDED.human_reason, abort_reason=EXCLUDED.abort_reason,
updated_at=EXCLUDED.updated_at, committed_at=EXCLUDED.committed_at, auto_ok=EXCLUDED.auto_ok, force_human=EXCLUDED.force_human`
	  var committed any
	  if p.CommittedAt.IsZero() {
		  committed = nil
	  } else {
		  committed = p.CommittedAt
	  }
	  return m.exec(ctx, q, pg,
		  p.ID, p.FailureID, p.TransferID, p.UserID, p.Asset, string(p.State), b01(p.AutoOK), b01(p.ForceHuman),
		   b01(p.LiveHash), b01(p.ChainBlocked), p.PolicyCode, p.PolicyVersion, p.IdempotencyKey, p.HumanTicketID,
		   p.HumanActor, p.HumanApprover, p.HumanReason, p.AbortReason, p.TimeoutAt, p.CreatedAt, p.UpdatedAt, committed,
	   )
   }

   func (m *mirror) upsertLeg(ctx context.Context, leg CompensateLeg) error {
	  q := `INSERT INTO compensate_legs (
id, plan_id, seq, kind, status, amount, asset, user_id, transfer_id, idempotency_key, result_ref,
reason_code, err_text, attempts, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE status=VALUES(status), result_ref=VALUES(result_ref), err_text=VALUES(err_text),
attempts=VALUES(attempts), updated_at=VALUES(updated_at)`
	   pg := `INSERT INTO compensate_legs ( 
id, plan_id, seq, kind, status, amount, asset, user_id, transfer_id, idempotency_key, result_ref,
reason_code, err_text, attempts, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, result_ref=EXCLUDED.result_ref, err_text=EXCLUDED.err_text,
attempts=EXCLUDED.attempts, updated_at=EXCLUDED.updated_at`
	   return m.exec(ctx, q, pg,
		  leg.ID,
		   leg.PlanID, leg.Seq, string(leg.Kind), string(leg.Status), leg.Amount, leg.Asset, leg.UserID,
		   leg.TransferID, leg.IdempotencyKey, leg.ResultRef, leg.ReasonCode, leg.ErrText, leg.Attempts, leg.CreatedAt, leg.UpdatedAt,
	  )
  }

   func (m *mirror) upsertEntry(ctx context.Context, e wdrollback.Entry) error {
	   q := `INSERT INTO rollback_entries (
id, plan_id, leg_id, failure_id, transfer_id, user_id, asset, kind, amount,
available_before, available_after, frozen_before, frozen_after, fee_before, fee_after,
idempotency_key, fingerprint, reason_code, path, applied, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE applied=VALUES(applied)`
	  pg := `INSERT INTO rollback_entries (
id, plan_id, leg_id, failure_id, transfer_id, user_id, asset, kind, amount,
available_before, available_after, frozen_before, frozen_after, fee_before, fee_after,
idempotency_key, fingerprint, reason_code, path, applied, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET applied=EXCLUDED.applied`
	  return m.exec(ctx, q, pg,
		   e.ID, e.PlanID, e.LegID, e.FailureID, e.TransferID, e.UserID, e.Asset, e.Kind, e.Amount,
		  e.AvailableBefore, e.AvailableAfter, e.FrozenBefore, e.FrozenAfter, e.FeeBefore, e.FeeAfter,
		   e.IdempotencyKey, e.Fingerprint, e.ReasonCode, e.Path, b01(e.Applied), e.CreatedAt,
	   )
  }

   func (m *mirror) upsertPolicy(ctx context.Context, p AutoPolicy) error {
	   legs := ""
	   for i, leg := range p.Legs {
		  if i > 0 {
			   legs += ","
		   }
		   legs += leg
	   }
	  q := `INSERT INTO auto_policies (
code, version, auto_allowed, force_human, dual_confirm, timeout_sec, leg_template, note, updated_by, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE version=VALUES(version), auto_allowed=VALUES(auto_allowed), force_human=VALUES(force_human),
dual_confirm=VALUES(dual_confirm), timeout_sec=VALUES(timeout_sec), leg_template=VALUES(leg_template),
note=VALUES(note), updated_by=VALUES(updated_by), updated_at=VALUES(updated_at)`
	   pg := `INSERT INTO auto_policies (
code, version, auto_allowed, force_human, dual_confirm, timeout_sec, leg_template, note, updated_by, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (code) DO UPDATE SET version=EXCLUDED.version, auto_allowed=EXCLUDED.auto_allowed, force_human=EXCLUDED.force_human,
dual_confirm=EXCLUDED.dual_confirm, timeout_sec=EXCLUDED.timeout_sec, leg_template=EXCLUDED.leg_template,
note=EXCLUDED.note, updated_by=EXCLUDED.updated_by, updated_at=EXCLUDED.updated_at`
	   return m.exec(ctx, q, pg,
		  string(p.Code), p.Version, b01(p.AutoAllowed), b01(p.ForceHuman), b01(p.DualConfirm), p.TimeoutSec,
		  legs,
		   p.Note, p.UpdatedBy, p.UpdatedAt,
	  )
  }
