package wdcomp

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/wdrollback"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func checkDual(actor, approver string) error {
	   actor = strings.TrimSpace(actor)
	   approver = strings.TrimSpace(approver)
	  if actor == "" || approver == "" {
		   return ErrDualApproval
	  }
	   if strings.EqualFold(actor, approver) {
		  return ErrDualApproval
	   }
	   if strings.EqualFold(actor, "system") || strings.EqualFold(approver, "system") {
		  return ErrDualApproval
	   }
	  return nil
  }

// Confirm 双人确认。
// 功能：记下操作人与复核人，并挂事故。
// release 只在非热态下把计划从 needs_human 拉回 executing。
// 不变量：热态下 release 被拒，冻结与可用保持原值。
  func (o *Orchestrator) Confirm(ctx context.Context, planID, actor, approver, reason string, release bool) (PlanView, error) {
	   if err := ctx.Err(); err != nil {
		   return PlanView{}, err
	   }
	   if o == nil {
		  return PlanView{}, ErrUnavailable
	  }
	  if err := checkDual(actor, approver); err != nil {
		  return PlanView{}, err
	  }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   now := o.now()
	   p := o.plans[strings.TrimSpace(planID)]
	   if p == nil {
		  return PlanView{}, ErrNotFound
	  }
	  if p.State != PlanNeedsHuman {
		  return o.view(p.ID), ErrState
	   }
	  f := o.fail[p.FailureID]
	   if f == nil {
		   return PlanView{}, ErrNotFound
	  }
	  p.HumanActor = strings.TrimSpace(actor)
	   p.HumanApprover = strings.TrimSpace(approver)
	   p.HumanReason = strings.TrimSpace(reason)
	  p.HumanAt = now
	  p.UpdatedAt = now
	  o.openIncident(p, f, "dual_ack", p.HumanActor, p.HumanApprover, reason, now)
	   if !release {
		   o.mirrorPlan(ctx, p)
		   return o.view(p.ID), nil
	   }
	  hot := p.LiveHash || p.ChainBlocked || f.LiveHash || f.ChainBlocked || !wdrollback.LiveUnfreezeAllowed() && (f.LiveHash || f.ChainBlocked)
	   if hot {
		   amt, _ := parseOpt(f.Compensable)
		   if !amt.IsPositive() {
			   amt, _ = parseOpt(f.FeeReserved)
		   }
		   _, err := o.svc.Apply(wdrollback.Request{
			   Kind: wdrollback.KindFreeze, PlanID: p.ID, FailureID: f.ID, TransferID: f.TransferID,
			   UserID: f.UserID, Asset: f.Asset, Amount: amt, Mode: wdrollback.ModeHuman,
			  LiveHash: true, ChainBlocked: f.ChainBlocked || p.ChainBlocked,
			  IdempotencyKey: p.ID + ":human-release", ReasonCode: f.ReasonCode,
			  Path: "confirm_release", TxHash: f.TxHash, Now: now,
		  })
		  noteDeny()
		   o.mirrorPlan(ctx, p)
		  if err == nil {
			  return o.view(p.ID),
				   ErrAutoUnfreezeDenied
		   }
		   return o.view(p.ID), err
	   }
	  if err := o.transit(p, PlanExecuting, now); err != nil {
		   return o.view(p.ID), err
	   }
	   f.Status = FailCompensating
	  f.UpdatedAt = now
	  return o.execute(ctx, p.ID, now)
  }

// Abort 放弃尚未动账的计划。已经释放过冻结或费预留的计划不能静默作废。
  func (o *Orchestrator) Abort(ctx context.Context, planID, actor, reason string) (PlanView, error) {
	   if err := ctx.Err(); err != nil {
		   return PlanView{}, err
	   }
	   if o == nil {
		   return PlanView{}, ErrUnavailable
	  }
	   if strings.TrimSpace(actor) == "" || strings.EqualFold(actor, "system") {
		   return PlanView{}, ErrDualApproval
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   now := o.now()
	  p := o.plans[strings.TrimSpace(planID)]
	  if p == nil {
		   return PlanView{}, ErrNotFound
	   }
	   if p.State == PlanCommitted || p.State == PlanAborted {
		   return o.view(p.ID), ErrState
	  }
	   f := o.fail[p.FailureID]
	  if f != nil && o.moneyMoved(p, f) {
		  return o.view(p.ID),
			   ErrAbortPartial
	   }
	  if p.State == PlanOpen {
		  if err := o.transit(p, PlanPlanning, now); err != nil {
			   return o.view(p.ID), err
		   }
	  }
	  if err := o.transit(p, PlanAborted, now); err != nil {
		  return o.view(p.ID), err
	   }
	   p.AbortReason = strings.TrimSpace(reason)
	   for _, leg := range o.legs[p.ID] {
		   if leg.Status == LegDone || leg.Status == LegSkipped {
			  continue
		   }
		   leg.Status = LegRejected
		  leg.UpdatedAt = now
		  o.mirrorLeg(ctx, leg)
	   }
	  if f != nil {
		   f.Status = FailAborted
		   f.UpdatedAt = now
		  o.mirrorFailure(ctx, f)
	   }
	  o.mirrorPlan(ctx, p)
	  return o.view(p.ID), nil
   }

  func (o *Orchestrator) moneyMoved(p *CompensatePlan, f *WdFailure) bool {
	   for _, e := range o.svc.Entries(f.UserID, f.Asset, f.TransferID) {
		   if e.PlanID != p.ID || !e.Applied {
			   continue
		   }
		  if e.Kind == wdrollback.KindFreeze || e.Kind == wdrollback.KindFee {
			  return true
		  }
	  }
	   return false
  }

  func (o *Orchestrator) enterHuman(ctx context.Context, p *CompensatePlan, f *WdFailure, kind string, now time.Time) error {
	   if p == nil || f == nil {
		   return ErrState
	   }
	   if p.State == PlanNeedsHuman {
		  return nil
	  }
	   if p.State == PlanCommitted || p.State == PlanAborted {
		   return ErrState
	  }
	  if p.State == PlanOpen {
		  if err := o.transit(p, PlanPlanning, now); err != nil {
			  return err
		   }
	   }
	  if err := o.transit(p, PlanNeedsHuman, now); err != nil {
		  return err
	   }
	  f.Status = FailNeedsHuman
	   f.UpdatedAt = now
	   p.ForceHuman = true
	   p.AutoOK = false
	   if p.LiveHash || p.ChainBlocked || f.LiveHash || f.ChainBlocked {
		   o.blockMoneyLegs(p.ID, now)
	   }
	  o.openIncident(p, f, kind, "system", "", kind, now)
	  _, err := o.svc.Apply(wdrollback.Request{
		   Kind: wdrollback.KindDeny, PlanID: p.ID, FailureID: f.ID, TransferID: f.TransferID,
		   UserID: f.UserID, Asset: f.Asset, Amount: money.Zero(),
		   IdempotencyKey: p.ID + ":deny:" + kind, ReasonCode: f.ReasonCode, Path: kind,
		   TxHash: f.TxHash, Now: now,
	  })
	   if err == nil {
		   noteDeny()
	   }
	   o.mirrorPlan(ctx, p)
	  o.mirrorFailure(ctx, f)
	   o.mirrorLegs(ctx, p.ID)
	   return err
  }

  func (o *Orchestrator) blockMoneyLegs(planID string, now time.Time) {
	  for _, leg := range o.legs[planID] {
		   if leg.Status == LegDone || leg.Status == LegSkipped || leg.Status == LegRejected {
			   continue
		  }
		   if leg.Kind == LegAudit || leg.Kind == LegIncident {
			   leg.Status = LegDone
			   leg.UpdatedAt = now
			   continue
		   }
		  leg.Status = LegBlocked
		  leg.ErrText = "live or unknown chain; auto unfreeze blocked"
		  leg.UpdatedAt = now
	  }
  }

   func (o *Orchestrator) openIncident(p *CompensatePlan, f *WdFailure, kind, actor, approver, note string, now time.Time) {
	   inc := Incident{
		  ID: o.next("inc"), PlanID: p.ID, FailureID: f.ID, TransferID: f.TransferID, UserID: f.UserID,
		  Kind: kind, Actor: actor, Approver: approver, Note: strings.TrimSpace(note), OpenedAt: now,
	  }
	   if p.HumanTicketID == "" {
		   p.HumanTicketID = inc.ID
	  }
	   inc.WarRoomRef = "warroom:" + p.HumanTicketID
	  o.incidents = append(o.incidents, inc)
  }
