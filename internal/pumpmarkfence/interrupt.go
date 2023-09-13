package pumpmarkfence

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// DeliverInterrupt Publish 成功后进程崩溃未 Mark。
  func (e *Engine) DeliverInterrupt(ctx context.Context, ev persist.OutboxEvent, h persist.Handler, actor string) (*DeliverEffectGen, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	   if h == nil {
		  return nil, ErrHandler
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   actor = actorOr(actor)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  gen, err := e.currentOrIssue(ctx, ev, actor)
	  if err != nil {
		  return gen, err
	  }
	  if gen.State == GenMarked || gen.Applied {
		   _, _ = e.writeDup(ctx, gen, ProofTickReplay, actor, ReasonTickSecond)
		   return gen, ErrDupEffect
	  }
	   if gen.State == GenDelivered {
		  _ = e.ensureHalf(ctx, gen, ReasonPublishNoMark)
		   return gen, ErrHalfMarkOpen
	  }
	  if err := h(ctx, ev); err != nil {
		  return gen, ErrHandler
	  }
	   if err := TransitionGen(gen, GenDelivered); err != nil {
		   return gen, err
	   }
	  gen.DeliveredAt = e.now()
	   gen.HandlerInvoked = true
	   if err := e.gens.Save(ctx, gen); err != nil {
		  return gen, err
	  }
	  _ = e.ensureHalf(ctx, gen, ReasonPublishNoMark)
	  recordDeliver()
	  recordHalfOpen()
	   e.audit(ctx, gen.GenID, ev.ID, actor, ActionDeliver, OutcomeHalfOpen, ReasonPublishNoMark, gen.Generation)
	   return gen, ErrHalfMarkOpen
  }

// SimulatePeerUnacked Mark 已成功但对端未确认：半截仍 open 且 PeerAcked=false。
   func (e *Engine) SimulatePeerUnacked(ctx context.Context, outboxID, actor string) (*HalfMarkCase, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  gen, err := e.gens.GetByOutbox(ctx, outboxID)
	  if err != nil {
		   return nil, err
	   }
	   if gen.State != GenMarked {
		   return nil, ErrDeliveredOnly
	  }
	   half, err := e.halves.GetByGen(ctx, gen.GenID)
	   if err != nil {
		   half = &HalfMarkCase{
			  CaseID: newID(), GenID: gen.GenID, OutboxID: gen.OutboxID,
			   EffectKey: gen.EffectKey, State: HalfOpen, Reason: ReasonMarkNoAck, OpenedAt: e.now(),
		  }
	   } else {
		   half.State = HalfOpen
		  half.Reason = ReasonMarkNoAck
		   half.PeerAcked = false
		   half.ResumedAt = time.Time{}
	  }
	  if err := e.halves.Save(ctx, half); err != nil {
		   return nil, err
	   }
	  e.audit(ctx, gen.GenID, outboxID, actorOr(actor), ActionMark, OutcomeHalfOpen, ReasonMarkNoAck, gen.Generation)
	  return half, nil
  }

// AckPeer 对端确认半截案。
   func (e *Engine) AckPeer(ctx context.Context, outboxID, actor string) (*HalfMarkCase, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  gen, err := e.gens.GetByOutbox(ctx, outboxID)
	  if err != nil {
		  return nil, err
	  }
	  half, err := e.halves.GetByGen(ctx, gen.GenID)
	  if err != nil {
		   return nil, err
	   }
	   half.PeerAcked = true
	  if half.State.Open() && half.Reason == ReasonMarkNoAck {
		   if err := TransitionHalf(half, HalfResumed); err != nil {
			  return nil, err
		  }
		   half.ResumedAt = e.now()
	  }
	   if err := e.halves.Save(ctx, half); err != nil {
		  return nil, err
	   }
	  e.audit(ctx, half.GenID, outboxID, actorOr(actor), ActionResume, OutcomeOK, "peer ack", gen.Generation)
	  return half, nil
  }
