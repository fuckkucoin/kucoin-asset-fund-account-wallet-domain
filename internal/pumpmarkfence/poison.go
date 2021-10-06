package pumpmarkfence

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

// PoisonReplayReq 运维毒丸重投断言。
type PoisonReplayReq struct {
	OutboxID string `json:"outbox_id"`
	ExpectGen int64  `json:"expect_generation"`
	Actor     string `json:"actor"`
	Payload   string `json:"payload,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Key          string `json:"key,omitempty"`
	AllowHandler bool   `json:"allow_handler"`
}

// PoisonReplayResult 重投闸结果。

type PoisonReplayResult struct {
	OK bool `json:"ok"`
	Gen   *DeliverEffectGen `json:"deliver_effect_gen,omitempty"`
	Proof *OutboxDupProof   `json:"dup_proof,omitempty"`
	Resumed bool `json:"resumed"`
}

// PoisonReplayAssert 毒丸第二腿。已 marked 必须失败。

   func (e *Engine) PoisonReplayAssert(ctx context.Context, req PoisonReplayReq, h persist.Handler) (PoisonReplayResult, error) {
	  out := PoisonReplayResult{}
	   if e == nil {
		  return out, ErrUnavailable
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	  outboxID := strings.TrimSpace(req.OutboxID)
	   if outboxID == "" {
		   return out, ErrInvalidInput
	  }
	  actor := actorOr(req.Actor)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  ev,
		  err := e.box.Get(ctx, outboxID)
	  if err != nil {
		   return out, ErrOutbox
	   }
	  kind, key, payload := ev.Kind, ev.Key, ev.Payload
	   if strings.TrimSpace(req.Kind) != "" {
		  kind = req.Kind
	  }
	   if strings.TrimSpace(req.Key) != "" {
		  key = req.Key
	   }
	   if strings.TrimSpace(req.Payload) != "" {
		   payload = req.Payload
	   }
	   ph := PayloadHash(kind, key, payload)
	   gen, gerr := e.gens.GetByOutbox(ctx, outboxID)
	   if gerr != nil {
		   stub := &DeliverEffectGen{OutboxID: outboxID, PayloadHash: ph, EffectKey: EffectKey(outboxID, ph)}
		   p, _ := e.writeDup(ctx, stub, ProofMarkNoGen, actor, "poison replay without gen")
		  out.Proof = p
		   e.audit(ctx, "", outboxID, actor, ActionPoisonReplay, OutcomeDenied, "no gen", 0)
		  recordPoisonDeny("no_gen")
		  return out, ErrGenMissing
	  }
	   out.Gen = gen
	   if applied, aerr := e.applied.Get(ctx, gen.EffectKey); aerr == nil && applied != nil {
		   p, _ := e.writeDup(ctx, gen, ProofPoisonReplay, actor, ReasonPoisonSecond)
		  out.Proof = p
		  e.audit(ctx, gen.GenID, outboxID, actor, ActionPoisonReplay, OutcomeDup, ReasonPoisonSecond, gen.Generation)
		  recordPoisonDeny("applied")
		  return out, ErrPoisonReplay
	   }
	   if gen.State == GenMarked || gen.Applied {
		   p, _ := e.writeDup(ctx, gen, ProofPoisonReplay, actor, ReasonPoisonSecond)
		   out.Proof = p
		   recordPoisonDeny("marked")
		   return out, ErrPoisonReplay
	  }
	   if err := FailClosedReplay(gen, req.ExpectGen, e.now(), ph); err != nil {
		   kind := ClassifyFail(err)
		   if err == ErrPoisonReplay {
			   kind = ProofPoisonReplay
		  }
		   p, _ := e.writeDup(ctx, gen, kind, actor, err.Error())
		   out.Proof = p
		  e.audit(ctx, gen.GenID, outboxID, actor, ActionPoisonReplay, OutcomeDenied, err.Error(), gen.Generation)
		  recordPoisonDeny("fail_closed")
		  return out, err
	   }
	  if err := CASHold(gen, req.ExpectGen); err != nil {
		  p, _ := e.writeDup(ctx, gen, ProofPoisonReplay, actor, err.Error())
		   out.Proof = p
		  recordPoisonDeny("cas")
		   return out, err
	   }
	   if gen.State == GenDelivered {
		   half,
			  _ := e.halves.GetOpenByOutbox(ctx, outboxID)
		  if half == nil {
			  half = e.ensureHalf(ctx, gen, ReasonPublishNoMark)
		  }
		   if err := e.resumeMarkLocked(ctx, ev, gen, half, actor, ""); err != nil {
			  if isDupErr(err) {
				   p, _ := e.writeDup(ctx, gen, ProofPoisonReplay, actor, err.Error())
				   out.Proof = p
			   }
			  return out, err
		   }
		   out.OK = true
		   out.Resumed = true
		   out.Gen = gen
		  recordPoisonOK("resume")
		   return out, nil
	  }
	  if !req.AllowHandler || h == nil {
		   e.audit(ctx, gen.GenID, outboxID, actor, ActionPoisonReplay, OutcomeDenied, "handler not allowed", gen.Generation)
		  recordPoisonDeny("no_handler")
		   return out, ErrHandler
	   }
	  if err := CASBump(gen, req.ExpectGen); err != nil {
		  return out, err
	  }
	  if err := e.gens.Save(ctx, gen); err != nil {
		  return out, err
	   }
	   if err := h(ctx, ev); err != nil {
		   e.audit(ctx, gen.GenID, outboxID, actor, ActionPoisonReplay, OutcomeDenied, err.Error(), gen.Generation)
		  recordPoisonDeny("handler")
		   return out, ErrHandler
	  }
	  if err := TransitionGen(gen, GenDelivered); err != nil {
		  return out, err
	   }
	  gen.DeliveredAt = e.now()
	   gen.HandlerInvoked = true
	  if err := e.gens.Save(ctx, gen); err != nil {
		   return out, err
	  }
	  half := e.ensureHalf(ctx, gen, ReasonPublishNoMark)
	   if err := e.commitMarkLocked(ctx, ev, gen, half, actor); err != nil {
		  out.Gen = gen
		   return out, err
	   }
	  out.OK = true
	   out.Gen = gen
	  recordPoisonOK("deliver")
	   e.audit(ctx, gen.GenID, outboxID, actor, ActionPoisonReplay, OutcomeOK, "replay first leg", gen.Generation)
	  return out, nil
   }
