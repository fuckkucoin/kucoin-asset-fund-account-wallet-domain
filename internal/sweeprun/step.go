package sweeprun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func (o *Orch) transitPlan(p *SweepPlan, to PlanState, actor string, now time.Time) error {
	  if err := canPlan(p.State, to); err != nil {
		   return err
	   }
	  p.State = to
	   p.UpdatedAt = now
	   if err := o.appendLink(p, "-", 0, kindPlan, "", "", "0", string(to), actor, RefBind{}, now); err != nil {
		   return err
	   }
	   metrics.SweepProof.WithLabelValues(string(to)).Inc()
	  return nil
  }

  func (o *Orch) transitLeg(p *SweepPlan, legID string, to LegState, actor string, now time.Time) error {
	   lg := p.legByID(legID)
	   if lg == nil {
		   return ErrLegNotFound
	   }
	   if err := ColdMustWait(lg.Cold, lg.State, to); err != nil {
		  return err
	  }
	   if err := canLeg(lg.State, to); err != nil {
		   return err
	  }
	   refs := RefBind{}
	  if lg.Cold && coldStateNeedsRef(to) {
		   if !lg.Refs.Bound() {
			   return ErrColdRef
		  }
		  refs = lg.Refs
	   }
	   lg.State = to
	  lg.UpdatedAt = now
	   if err := o.appendLink(p, lg.ID, lg.Seq, lg.Kind, lg.From, lg.To, lg.Amount, string(to), actor, refs, now); err != nil {
		  return err
	   }
	   metrics.SweepProof.WithLabelValues("leg_" + string(to)).Inc()
	  return nil
  }

   func coldStateNeedsRef(to LegState) bool {
	   switch to {
	  case LegReady, LegExecuting, LegDone, LegCompensated:
		   return true
	  default:
		   return false
	   }
   }

   func (o *Orch) Tick(ctx context.Context, limit int) (TickReport, error) {
	   if o == nil {
		   return TickReport{}, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 32
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  if o.loadErr != nil {
		  return TickReport{Halted: true},
			  ErrHalted
	  }
	   if o.halt {
		  return TickReport{Halted: true}, ErrHalted
	   }
	  var rep TickReport
	   for _, id := range o.orderedIDs() {
		   if rep.Advanced >= limit {
			   break
		  }
		   p := o.plans[id]
		  if p == nil || p.State.Terminal() || p.State == PlanStateHalted || p.State == PlanStateDraft {
			  continue
		   }
		  moved, waited, err := o.step(p)
		  if err != nil {
			  if errors.Is(err, ErrChainBreak) {
				   o.halt = true
			   }
			  return rep, err
		   }
		  if waited {
			  rep.Waited++
		  }
		  if moved {
			  rep.Advanced++
			  rep.Plans = append(rep.Plans, id)
			  if err := o.save(ctx, id); err != nil {
				   return rep, err
			   }
		  }
	   }
	   return rep, nil
  }

   func (o *Orch) step(p *SweepPlan) (moved, waited bool, err error) {
	  now := o.now()
	  switch p.State {
	   case PlanStateGated:
		   if !p.ColdDeadline.IsZero() && !now.Before(p.ColdDeadline) && !p.coldSatisfied() {
			  if err := o.rejectUnbound(p, now); err != nil {
				  return false, false, err
			  }
			  return true, false, nil
		  }
		  if !p.coldSatisfied() {
			   return false, true, nil
		  }
		  if err := o.transitPlan(p, PlanStateArmed, laneActor, now); err != nil {
			  return false, false, err
		   }
		  return true, false, nil
	   case PlanStateArmed:
		   if err := o.transitPlan(p, PlanStateRunning, laneActor, now); err != nil {
			   return false, false, err
		   }
		  return true, false, nil
	  case PlanStateRunning, PlanStatePartial, PlanStateCompensating:
		  return o.stepLeg(p, now)
	   default:
		   return false, false, ErrState
	  }
  }

  func (o *Orch) stepLeg(p *SweepPlan, now time.Time) (bool, bool, error) {
	  lg := p.activeLeg()
	  if lg == nil {
		   if err := o.finishIdle(p, now); err != nil {
			   return false, false, err
		  }
		  return true, false, nil
	   }
	  id := lg.ID
	  switch lg.State {
	   case LegPending:
		   if err := o.transitLeg(p, id, LegProofOpen, laneActor, now); err != nil {
			   return false, false, err
		   }
		  return true, false, nil
	  case LegProofOpen:
		   next := LegReady
		   if lg.Cold {
			  next = LegCeremonyWait
		   }
		   if err := o.transitLeg(p, id, next, laneActor, now); err != nil {
			   return false, false, err
		   }
		  return true, false, nil
	   case LegCeremonyWait:
		   cur := p.legByID(id)
		  if cur == nil {
			   return false, false, ErrLegNotFound
		   }
		  if !cur.Refs.Bound() {
			   if !p.ColdDeadline.IsZero() && !now.Before(p.ColdDeadline) {
				  if err := o.failLeg(p, id, "cold ref timeout", now); err != nil {
					   return false, false, err
				  }
				  return true, false, nil
			  }
			   return false, true, nil
		   }
		  verdict, err := o.judgeRefs(p, cur, cur.Refs, now)
		  if err != nil {
			   if ferr := o.failLeg(p, id, err.Error(), now); ferr != nil {
				   return false, false, ferr
			   }
			  return true, false, nil
		  }
		   cur.Refs.CeremonyPhase = verdict.Phase
		  cur.Refs.AirgapPhase = verdict.Airgap
		   cur.Refs.SignState = verdict.Sign
		  if err := o.transitLeg(p, id, LegReady, laneActor, now); err != nil {
			  return false, false, err
		  }
		   return true, false, nil
	   case LegReady:
		   if err := o.transitLeg(p, id, LegExecuting, laneActor, now); err != nil {
			  return false, false, err
		  }
		  return true, false, nil
	   case LegExecuting:
		  cur := p.legByID(id)
		  if cur == nil {
			  return false, false, ErrLegNotFound
		  }
		  if o.failNext != "" {
			   why := o.failNext
			   o.failNext = ""
			   if err := o.failLeg(p, id, why, now); err != nil {
				   return false, false, err
			   }
			   return true, false, nil
		   }
		  if !cur.Compensate {
			  if err := o.agreeForward(p, cur); err != nil {
				   if ferr := o.failLeg(p, id, err.Error(), now); ferr != nil {
					  return false, false, ferr
				   }
				   return true, false, nil
			   }
		  }
		   if err := o.desk.Move(p.Asset, cur.From, cur.To, cur.Amount); err != nil {
			  if ferr := o.failLeg(p, id, err.Error(), now); ferr != nil {
				  return false, false, ferr
			   }
			  return true, false, nil
		   }
		  if err := o.transitLeg(p, id, LegDone, laneActor, now); err != nil {
			   if rerr := o.desk.Move(p.Asset, cur.To, cur.From, cur.Amount); rerr != nil {
				  o.halt = true
				  o.loadErr = rerr
				  return false, false, fmt.Errorf("%w: inventory rollback failed: %v", err, rerr)
			   }
			   return false, false, err
		   }
		   if err := o.afterDone(p, id, now); err != nil {
			   return false, false, err
		  }
		   return true, false, nil
	   default:
		  return false, false, ErrState
	  }
  }

   func (o *Orch) afterDone(p *SweepPlan, legID string, now time.Time) error {
	  lg := p.legByID(legID)
	   if lg == nil {
		  return ErrLegNotFound
	  }
	  if lg.Compensate {
		   return o.finishCompLeg(p, legID, now)
	  }
	  if p.forwardOpen() {
		   if p.State == PlanStateRunning {
			  return o.transitPlan(p, PlanStatePartial, laneActor, now)
		   }
		  return nil
	   }
	   p.Code = "settled"
	  return o.transitPlan(p, PlanStateSettled, laneActor, now)
   }

   func (o *Orch) finishIdle(p *SweepPlan, now time.Time) error {
	  if p.State == PlanStateCompensating && compLegsDone(p) {
		   if err := o.releaseHold(p); err != nil {
			   return err
		  }
		  p.Code = "compensated"
		  return o.transitPlan(p, PlanStateCompensated, laneActor, now)
	  }
	  if !p.forwardOpen() && p.State != PlanStateCompensating {
		   p.Code = "settled"
		   return o.transitPlan(p, PlanStateSettled, laneActor, now)
	  }
	   return ErrState
  }

   func (o *Orch) rejectUnbound(p *SweepPlan, now time.Time) error {
	  ids := make([]string, 0)
	  for i := range p.Legs {
		  lg := &p.Legs[i]
		  if lg.Cold && !lg.Refs.Bound() && !lg.State.Quiet() {
			  ids = append(ids, lg.ID)
		   }
	   }
	   for _, id := range ids {
		   if err := o.transitLeg(p, id, LegFailed, laneActor, now); err != nil {
			   return err
		  }
		   if cur := p.legByID(id); cur != nil {
			  cur.Reason = "cold ref timeout"
		   }
	  }
	   p.Reason = "cold ref timeout"
	   p.Code = "deny_cold_ref"
	   metrics.SweepProof.WithLabelValues("cold_deny").Inc()
	   if err := o.releaseHold(p); err != nil {
		   return err
	  }
	  return o.transitPlan(p, PlanStateRejected, laneActor, now)
  }

   func (o *Orch) AttachRefs(ctx context.Context, planID, legID, actor string, refs RefBind) (SweepPlan, error) {
	  if o == nil {
		   return SweepPlan{}, ErrUnavailable
	  }
	  who, err := validate.Actor(actor)
	  if err != nil {
		  return SweepPlan{}, ErrActor
	   }
	   refs.CeremonyID = strings.TrimSpace(refs.CeremonyID)
	  refs.AirgapID = strings.TrimSpace(refs.AirgapID)
	  refs.SignID = strings.TrimSpace(refs.SignID)
	  if !refs.Bound() {
		  return SweepPlan{}, ErrColdRef
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  p := o.plans[strings.TrimSpace(planID)]
	   if p == nil {
		   return SweepPlan{}, ErrPlanNotFound
	  }
	   if p.State.Terminal() || p.State == PlanStateHalted {
		   return p.Copy(), ErrTerminal
	   }
	  lg := p.legByID(strings.TrimSpace(legID))
	   if lg == nil {
		   return SweepPlan{},
			   ErrLegNotFound
	  }
	  if !lg.Cold {
		  return SweepPlan{}, ErrRefNotCold
	   }
	   switch lg.State {
	   case LegPending, LegProofOpen, LegCeremonyWait:
	  default:
		   return SweepPlan{}, ErrState
	  }
	  verdict, err := o.judgeRefs(p, lg, refs, o.now())
	   if err != nil {
		  metrics.SweepProof.WithLabelValues("cold_deny").Inc()
		   return p.Copy(), err
	   }
	   now := o.now()
	   lg.Refs = refs
	  lg.Refs.CeremonyPhase = verdict.Phase
	   lg.Refs.AirgapPhase = verdict.Airgap
	  lg.Refs.SignState = verdict.Sign
	   lg.Refs.BoundBy = who
	   lg.Refs.BoundAt = now
	  lg.UpdatedAt = now
	   if err := o.appendLink(p, lg.ID, lg.Seq, lg.Kind, lg.From, lg.To, lg.Amount, "ref_bound", who, lg.Refs, now); err != nil {
		   lg.Refs = RefBind{}
		   return SweepPlan{}, err
	  }
	   if err := o.save(ctx, p.ID); err != nil {
		  return p.Copy(), err
	   }
	   return p.Copy(), nil
   }

   func (o *Orch) Halt(ctx context.Context, planID, actor string) (SweepPlan, error) {
	  if o == nil {
		  return SweepPlan{}, ErrUnavailable
	  }
	   if _, err := validate.Actor(actor); err != nil {
		  return SweepPlan{}, ErrActor
	   }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	   now := o.now()
	  if strings.TrimSpace(planID) == "" {
		   o.halt = true
		  metrics.SweepProof.WithLabelValues("halt").Inc()
		  return SweepPlan{}, nil
	   }
	  p := o.plans[strings.TrimSpace(planID)]
	   if p == nil {
		   return SweepPlan{}, ErrPlanNotFound
	   }
	   if p.State.Terminal() {
		   return p.Copy(), ErrTerminal
	  }
	   if p.State == PlanStateHalted {
		   return p.Copy(), nil
	   }
	  if err := o.transitPlan(p, PlanStateHalted, actor, now); err != nil {
		   return p.Copy(), err
	  }
	  if err := o.save(ctx, p.ID); err != nil {
		   return p.Copy(), err
	  }
	   return p.Copy(), nil
  }

   func (o *Orch) Resume(ctx context.Context, planID, actor string) (SweepPlan, error) {
	  if o == nil {
		   return SweepPlan{}, ErrUnavailable
	   }
	  who, err := validate.Actor(actor)
	   if err != nil {
		  return SweepPlan{}, ErrActor
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  now := o.now()
	  if strings.TrimSpace(planID) == "" {
		  o.halt = false
		   metrics.SweepProof.WithLabelValues("resume").Inc()
		   return SweepPlan{}, nil
	   }
	  p := o.plans[strings.TrimSpace(planID)]
	  if p == nil {
		  return SweepPlan{}, ErrPlanNotFound
	   }
	  if p.State != PlanStateHalted {
		  return p.Copy(), ErrState
	  }
	  p.ColdDeadline = now.Add(o.coldWait)
	  if err := o.transitPlan(p, PlanStateGated, who, now); err != nil {
		   return p.Copy(), err
	   }
	   if err := o.save(ctx, p.ID); err != nil {
		   return p.Copy(), err
	   }
	  return p.Copy(), nil
   }

   func (o *Orch) Halted() bool {
	  if o == nil {
		   return true
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   return o.halt || o.loadErr != nil
  }

// Balance 读影子库存，给运维对账。不读用户账。

  func (o *Orch) Balance(asset string) (treasury.Bal, error) {
	  if o == nil || o.desk == nil {
		  return treasury.Bal{}, ErrUnavailable
	  }
	  return o.desk.Get(asset)
   }
