package pumpmarkfence

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TickWithGen 一轮带世代的泵。h 为全额业务效果（Publish/Handler）。
  func (e *Engine) TickWithGen(ctx context.Context, h persist.Handler, actor string) (TickReport, error) {
	   return e.TickWithGenLimit(ctx, h, actor, DefaultTickLimit)
  }

  func (e *Engine) TickWithGenLimit(ctx context.Context, h persist.Handler, actor string, limit int) (TickReport, error) {
	  out := TickReport{}
	  if e == nil || e.box == nil {
		   out.Err = ErrUnavailable.Error()
		  return out, ErrUnavailable
	  }
	   if h == nil {
		  out.Err = ErrHandler.Error()
		   return out, ErrHandler
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   actor = actorOr(actor)
	  if limit <= 0 {
		   limit = DefaultTickLimit
	  }
	  pend,
		  err := e.box.Pending(ctx, limit)
	  if err != nil {
		  out.Err = err.Error()
		  return out, err
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  var first error
	  for _, ev := range pend {
		  rep, err := e.tickOneLocked(ctx, ev, h, actor)
		   out.Delivered += rep.Delivered
		  out.Marked += rep.Marked
		  out.Resumed += rep.Resumed
		  out.DupProofs += rep.DupProofs
		  out.HalfOpened += rep.HalfOpened
		  out.HandlerCalls += rep.HandlerCalls
		   out.SkippedExpired += rep.SkippedExpired
		   out.GenIDs = append(out.GenIDs,
			  rep.GenIDs...)
		  if err != nil && first == nil && !isDupErr(err) && !errors.Is(err, ErrGenExpired) && !errors.Is(err, ErrFingerprint) {
			   if err != ErrHandler {
				   first = err
			   } else if first == nil {
				  first = err
			  }
		  }
		  if err == ErrHandler && out.Err == "" {
			   out.Err = err.Error()
			  if first == nil {
				  first = err
			   }
		   }
	   }
	  left,
		  _ := e.box.Pending(ctx, limit)
	  out.Pending = len(left)
	   if out.Err == "" && first != nil {
		  out.Err = first.Error()
	   }
	   return out, first
  }

   func (e *Engine) tickOneLocked(ctx context.Context, ev persist.OutboxEvent, h persist.Handler, actor string) (TickReport, error) {
	   rep := TickReport{}
	   gen, err := e.currentOrIssue(ctx, ev, actor)
	  if err == ErrAlreadyApplied {
		   _, _ = e.writeDup(ctx, gen, ProofTickReplay, actor, ReasonTickSecond)
		   rep.DupProofs++
		   if gen != nil {
			   rep.GenIDs = append(rep.GenIDs,
				   gen.GenID)
		   }
		  return rep, ErrDupEffect
	  }
	   if err == ErrGenExpired {
		  _, _ = e.writeDup(ctx, gen, ProofExpired, actor, "ttl")
		  rep.DupProofs++
		   rep.SkippedExpired++
		  if gen != nil {
			  rep.GenIDs = append(rep.GenIDs, gen.GenID)
		   }
		  return rep, ErrGenExpired
	   }
	  if err == ErrFingerprint {
		  _, _ = e.writeDup(ctx, gen, ProofFingerprint, actor, "payload drift")
		  rep.DupProofs++
		  if gen != nil {
			  rep.GenIDs = append(rep.GenIDs, gen.GenID)
		  }
		  return rep, ErrFingerprint
	  }
	  if err != nil {
		   return rep, err
	  }
	  rep.GenIDs = append(rep.GenIDs, gen.GenID)
	   if gen.State == GenMarked || gen.Applied {
		  _, _ = e.writeDup(ctx, gen, ProofTickReplay, actor, ReasonTickSecond)
		  rep.DupProofs++
		  return rep, ErrDupEffect
	   }
	  half, herr := e.halves.GetOpenByOutbox(ctx, ev.ID)
	  if herr == nil && half != nil && gen.State == GenDelivered {
		   if err := e.resumeMarkLocked(ctx, ev, gen, half, actor, ""); err != nil {
			   if isDupErr(err) {
				  _, _ = e.writeDup(ctx, gen, ProofHalfSecond, actor, err.Error())
				  rep.DupProofs++
			   }
			  return rep, err
		   }
		   rep.Resumed++
		  rep.Marked++
		   return rep, nil
	   }
	  if gen.State == GenDelivered {
		   half = e.ensureHalf(ctx, gen, ReasonPublishNoMark)
		   if err := e.resumeMarkLocked(ctx, ev, gen, half, actor, ""); err != nil {
			  if isDupErr(err) {
				   _, _ = e.writeDup(ctx, gen, ProofHalfSecond, actor, err.Error())
				  rep.DupProofs++
			   }
			  return rep, err
		  }
		   rep.Resumed++
		   rep.Marked++
		   return rep, nil
	   }
	   if gen.State != GenIssued {
		   return rep, ErrIllegalTransit
	  }
	   if err := h(ctx, ev); err != nil {
		   e.audit(ctx, gen.GenID, ev.ID, actor, ActionDeliver, OutcomeDenied, err.Error(), gen.Generation)
		  recordHandlerFail()
		   return rep, ErrHandler
	  }
	   rep.HandlerCalls++
	   gen.HandlerInvoked = true
	  if err := TransitionGen(gen, GenDelivered); err != nil {
		   return rep, err
	   }
	   gen.DeliveredAt = e.now()
	  if err := e.gens.Save(ctx, gen); err != nil {
		  return rep, err
	   }
	   recordDeliver()
	   e.audit(ctx, gen.GenID, ev.ID, actor, ActionDeliver, OutcomeOK, "handler", gen.Generation)
	   rep.Delivered++
	   half = e.ensureHalf(ctx, gen, ReasonPublishNoMark)
	  if half != nil && half.State.Open() {
		   rep.HalfOpened++
	   }
	   if err := e.commitMarkLocked(ctx, ev, gen, half, actor); err != nil {
		  e.audit(ctx, gen.GenID, ev.ID, actor, ActionMark, OutcomeHalfOpen, err.Error(), gen.Generation)
		  recordHalfOpen()
		  return rep, err
	  }
	   rep.Marked++
	  return rep, nil
  }

  func (e *Engine) ensureHalf(ctx context.Context, gen *DeliverEffectGen, reason string) *HalfMarkCase {
	  if gen == nil {
		  return nil
	  }
	  if cur, err := e.halves.GetOpenByOutbox(ctx, gen.OutboxID); err == nil && cur != nil {
		  return cur
	   }
	  row := &HalfMarkCase{
		   CaseID: newID(), GenID: gen.GenID, OutboxID: gen.OutboxID,
		  EffectKey: gen.EffectKey,
		  State: HalfOpen, Reason: reason, OpenedAt: e.now(),
	   }
	   if err := e.halves.Save(ctx, row); err != nil {
		  return nil
	  }
	   return row
  }

  func newID() string {
	  return uuid.NewString()
   }
