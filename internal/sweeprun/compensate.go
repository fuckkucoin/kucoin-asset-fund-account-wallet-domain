package sweeprun

import (
	"fmt"
	"time"
	"github.com/google/uuid"
)

   func (o *Orch) failLeg(p *SweepPlan, legID, reason string, now time.Time) error {
	  lg := p.legByID(legID)
	  if lg == nil {
		  return ErrLegNotFound
	  }
	   lg.Reason = clipReason(reason)
	   if err := o.transitLeg(p, lg.ID, LegFailed, laneActor, now); err != nil {
		   return err
	   }
	  lg = p.legByID(legID)
	  if lg == nil {
		  return ErrLegNotFound
	   }
	   if lg.Compensate {
		   p.Reason = clipReason("compensate failed: " + reason)
		  p.Code = "compensate_failed"
		  return nil
	  }
	   done := p.doneForward()
	  if len(done) == 0 {
		   p.Reason = clipReason(reason)
		   p.Code = "leg_failed"
		  if err := o.releaseHold(p); err != nil {
			   return err
		   }
		   return o.transitPlan(p, PlanStateRejected, laneActor, now)
	   }
	   if err := o.openComp(p, done, reason, now); err != nil {
		   return err
	  }
	   p.Reason = clipReason(reason)
	  p.Code = "compensate"
	   if p.State == PlanStateCompensating {
		  return nil
	   }
	   return o.transitPlan(p, PlanStateCompensating, laneActor, now)
  }

  func (o *Orch) openComp(p *SweepPlan, doneIDs []string, reason string, now time.Time) error {
	  // 倒序：后完成的先冲回，影子库存才能回到开腿前。
	   for i := len(doneIDs) - 1; i >= 0; i-- {
		  parent := p.legByID(doneIDs[i])
		   if parent == nil {
			  return ErrLegNotFound
		   }
		  if parent.State != LegDone {
			  continue
		   }
		   parentID := parent.ID
		  if err := o.transitLeg(p, parentID, LegCompensating, laneActor, now); err != nil {
			   return err
		  }
		  parent = p.legByID(parentID)
		  seq := len(p.Legs) + 1
		  comp := SweepLeg{
			   ID: uuid.NewString(), PlanID: p.ID, Seq: seq, Kind: kindComp, SrcKind: parent.Kind,
			  From: parent.To, To: parent.From, Amount: parent.Amount,
			  Cold:  parent.To == "cold" || parent.From == "cold",
			   State: LegPending, ParentID: parentID, Compensate: true,
			  Reason: clipReason(reason), CreatedAt: now, UpdatedAt: now,
		   }
		  p.Legs = append(p.Legs, comp)
		  rec := CompRec{
			  ID:     uuid.NewString(),
			  PlanID: p.ID, ParentLeg: parentID, CompLeg: comp.ID,
			  Amount: comp.Amount, State: string(LegPending), Reason: clipReason(reason), CreatedAt: now,
		   }
		   o.comps[p.ID] = append(o.comps[p.ID], rec)
		  if err := o.appendLink(p, comp.ID, comp.Seq, kindComp, comp.From, comp.To, comp.Amount, "compensate_open", laneActor, RefBind{}, now); err != nil {
			   return err
		  }
	  }
	  if len(o.comps[p.ID]) == 0 {
		   return fmt.Errorf("%w: no compensate leg", ErrNoLeg)
	   }
	  return nil
  }

  func (o *Orch) finishCompLeg(p *SweepPlan, compID string, now time.Time) error {
	   comp := p.legByID(compID)
	  if comp == nil || !comp.Compensate {
		  return ErrLegNotFound
	  }
	   parentID := comp.ParentID
	   if parent := p.legByID(parentID); parent != nil && parent.State == LegCompensating {
		   if err := o.transitLeg(p, parentID, LegCompensated, laneActor, now); err != nil {
			   return err
		   }
	  }
	   for i := range o.comps[p.ID] {
		  if o.comps[p.ID][i].CompLeg == compID {
			   o.comps[p.ID][i].State = string(LegDone)
		   }
	  }
	   if !compLegsDone(p) {
		   return nil
	   }
	  if err := o.releaseHold(p); err != nil {
		  return err
	   }
	   p.Code = "compensated"
	  return o.transitPlan(p,
		   PlanStateCompensated, laneActor, now)
  }

  func compLegsDone(p *SweepPlan) bool {
	   saw := false
	   for i := range p.Legs {
		   if !p.Legs[i].Compensate {
			   continue
		  }
		   saw = true
		  if p.Legs[i].State != LegDone {
			   return false
		  }
	  }
	  return saw
  }
