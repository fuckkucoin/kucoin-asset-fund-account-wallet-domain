package pumpmarkfence

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func (e *Engine) commitMarkLocked(ctx context.Context, ev persist.OutboxEvent, gen *DeliverEffectGen, half *HalfMarkCase, actor string) error {
	  ph := PayloadHash(ev.Kind, ev.Key, ev.Payload)
	   if err := FailClosedMark(gen, e.now(), ph, false); err != nil {
		  return err
	   }
	   if gen.State == GenIssued {
		  if err := TransitionGen(gen, GenDelivered); err != nil {
			   return err
		  }
		   gen.DeliveredAt = e.now()
	   }
	   if err := e.box.Mark(ctx, ev.ID); err != nil {
		  return err
	   }
	  if err := TransitionGen(gen, GenMarked); err != nil {
		   return err
	   }
	   gen.MarkedAt = e.now()
	   if err := e.putApplied(ctx, gen); err != nil {
		   if err == ErrAlreadyApplied {
			   return ErrAlreadyApplied
		  }
		  return err
	   }
	   if err := e.gens.Save(ctx, gen); err != nil {
		  return err
	   }
	  if half != nil && half.State.Open() {
		   if err := TransitionHalf(half, HalfResumed); err != nil {
			  return err
		   }
		   half.ResumedAt = e.now()
		   half.ResumeCount++
		  half.PeerAcked = true
		   _ = e.halves.Save(ctx, half)
	  }
	  recordMark()
	   e.audit(ctx, gen.GenID, ev.ID, actor, ActionMark, OutcomeOK, "marked", gen.Generation)
	  return nil
  }

  func (e *Engine) resumeMarkLocked(ctx context.Context, ev persist.OutboxEvent, gen *DeliverEffectGen, half *HalfMarkCase, actor, token string) error {
	  if gen == nil {
		  return ErrGenMissing
	   }
	   if gen.State != GenDelivered {
		   return ErrDeliveredOnly
	   }
	   if half == nil {
		  return ErrHalfMarkNotFound
	  }
	  now := e.now()
	  holder := actorOr(actor)
	  if err := half.HoldExclusive(now, holder, token); err != nil {
		   return err
	   }
	   if half.ResumeCount >= 1 {
		   return ErrAlreadyApplied
	  }
	   half.ResumeHolder = holder
	   half.ResumeToken = ResumeToken(half.CaseID, gen.GenID, holder)
	   half.ResumeUntil = now.Add(ResumeHoldTTL)
	  if err := e.halves.Save(ctx, half); err != nil {
		   return err
	  }
	  if err := e.commitMarkLocked(ctx, ev, gen, half, holder); err != nil {
		   return err
	   }
	  recordResume()
	   e.audit(ctx, gen.GenID, ev.ID, holder, ActionResume, OutcomeResumed, "ops resume", gen.Generation)
	   return nil
   }

// ResumeMarkReq 运维续提。
type ResumeMarkReq struct {
	OutboxID string `json:"outbox_id"`
	GenID    string `json:"deliver_effect_gen"`
	Actor       string `json:"actor"`
	ResumeToken string `json:"resume_token"`
}

// ResumeMark 运维对半截案续提 Mark，不调用 handler。

   func (e *Engine) ResumeMark(ctx context.Context, req ResumeMarkReq) (*DeliverEffectGen, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   outboxID := strings.TrimSpace(req.OutboxID)
	   if outboxID == "" {
		   return nil, ErrInvalidInput
	   }
	   actor := actorOr(req.Actor)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   ev, err := e.box.Get(ctx, outboxID)
	   if err != nil {
		  return nil, ErrOutbox
	  }
	  gen, err := e.gens.GetByOutbox(ctx, outboxID)
	  if err != nil {
		   return nil, err
	  }
	  if strings.TrimSpace(req.GenID) != "" && req.GenID != gen.GenID {
		  return nil, ErrGenMismatch
	   }
	   half, err := e.halves.GetOpenByOutbox(ctx, outboxID)
	  if err != nil {
		  return nil, err
	  }
	   if err := e.resumeMarkLocked(ctx, ev, gen, half, actor, strings.TrimSpace(req.ResumeToken)); err != nil {
		  if isDupErr(err) {
			   _, _ = e.writeDup(ctx, gen, ProofHalfSecond, actor, err.Error())
		  }
		  return gen, err
	   }
	  return gen, nil
  }

// AbandonHalf 放弃半截案（世代过期后）。
   func (e *Engine) AbandonHalf(ctx context.Context, outboxID, actor, note string) (*HalfMarkCase, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	   outboxID = strings.TrimSpace(outboxID)
	  if outboxID == "" {
		  return nil, ErrInvalidInput
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   half, err := e.halves.GetOpenByOutbox(ctx, outboxID)
	  if err != nil {
		   return nil, err
	  }
	   gen, _ := e.gens.Get(ctx, half.GenID)
	  if gen != nil && gen.State == GenMarked {
		   return nil, ErrAlreadyApplied
	  }
	   if err := TransitionHalf(half, HalfAbandoned); err != nil {
		  return nil, err
	  }
	   half.AbandonedAt = e.now()
	  half.Note = strings.TrimSpace(note)
	  if err := e.halves.Save(ctx, half); err != nil {
		  return nil, err
	  }
	  if gen != nil && gen.State.Live() {
		  _ = TransitionGen(gen, GenExpired)
		   gen.ExpiredAt = e.now()
		  _ = e.gens.Save(ctx, gen)
	   }
	  e.audit(ctx,
		   half.GenID, outboxID, actorOr(actor), ActionAbandon, OutcomeAbandoned, half.Note, 0)
	   recordAbandon()
	  return half, nil
  }

// MarkDeniedFromLag Lag 只读面不得 Mark。
   func (e *Engine) MarkDeniedFromLag(ctx context.Context, outboxID, actor string) error {
	   if e == nil {
		  return ErrUnavailable
	  }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   gen, err := e.gens.GetByOutbox(ctx, strings.TrimSpace(outboxID))
	   if err != nil {
		  gen = &DeliverEffectGen{OutboxID: outboxID}
	   }
	  if err := FailClosedMark(gen, e.now(), "", true); err != nil {
		   _, _ = e.writeDup(ctx, gen, ProofLagMark, actorOr(actor), "lag snapshot mark")
		  e.audit(ctx,
			  gen.GenID, outboxID, actorOr(actor), ActionLagMark, OutcomeLagRO, err.Error(), gen.Generation)
		  return err
	   }
	   return ErrLagReadonly
  }
