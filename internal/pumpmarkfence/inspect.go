package pumpmarkfence

import (
	"context"
)

// InspectReport 巡检结果。

type InspectReport struct {
	Scanned int `json:"scanned"`
	Resumed int `json:"resumed"`
	Expired int `json:"expired"`
	Abandoned int `json:"abandoned"`
	Proofs int `json:"proofs"`
}

// InspectHalf 扫描 open 半截案：未过期则续提 Mark，过期则 abandon。

   func (e *Engine) InspectHalf(ctx context.Context, limit int, actor string) (InspectReport, error) {
	  out := InspectReport{}
	   if e == nil {
		  return out, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	   if limit <= 0 {
		   limit = 80
	   }
	   actor = actorOr(actor)
	   open, err := e.halves.ListOpen(ctx, limit)
	   if err != nil {
		   return out, err
	   }
	  out.Scanned = len(open)
	  for i := range open {
		  half := open[i]
		   e.mu.Lock()
		   gen, gerr := e.gens.Get(ctx, half.GenID)
		  if gerr != nil || gen == nil {
			   e.mu.Unlock()
			   continue
		   }
		  if gen.Expired(e.now()) || gen.State == GenExpired {
			   _ = TransitionHalf(&half, HalfAbandoned)
			  half.AbandonedAt = e.now()
			   _ = e.halves.Save(ctx, &half)
			  if gen.State.Live() {
				   _ = TransitionGen(gen, GenExpired)
				  gen.ExpiredAt = e.now()
				   _ = e.gens.Save(ctx, gen)
			  }
			  out.Expired++
			   out.Abandoned++
			  e.audit(ctx, gen.GenID, half.OutboxID, actor, ActionInspect, OutcomeExpired, "inspect expire", gen.Generation)
			   e.mu.Unlock()
			   continue
		  }
		  if gen.State == GenMarked {
			   _ = TransitionHalf(&half, HalfResumed)
			  half.ResumedAt = e.now()
			   half.PeerAcked = true
			  _ = e.halves.Save(ctx, &half)
			   out.Resumed++
			  e.mu.Unlock()
			   continue
		  }
		  ev, evErr := e.box.Get(ctx, half.OutboxID)
		  if evErr != nil {
			  e.mu.Unlock()
			  continue
		   }
		   if ev.SentAt != nil && gen.State == GenDelivered {
			  if err := TransitionGen(gen, GenMarked); err == nil {
				   gen.MarkedAt = e.now()
				  _ = e.putApplied(ctx, gen)
				  _ = e.gens.Save(ctx, gen)
			  }
			  _ = TransitionHalf(&half, HalfResumed)
			   half.ResumedAt = e.now()
			  half.PeerAcked = true
			  _ = e.halves.Save(ctx, &half)
			   out.Resumed++
			   e.mu.Unlock()
			  continue
		   }
		  if gen.State == GenDelivered && ev.SentAt == nil {
			   if err := e.resumeMarkLocked(ctx, ev, gen, &half, actor, ""); err != nil {
				   if isDupErr(err) {
					   _, _ = e.writeDup(ctx, gen, ProofHalfSecond, actor, err.Error())
					   out.Proofs++
				  }
				   e.mu.Unlock()
				  continue
			   }
			  out.Resumed++
		  }
		  e.mu.Unlock()
	  }
	   e.audit(ctx, "", "", actor, ActionInspect, OutcomeOK, "batch", 0)
	  return out, nil
  }

// ExpireStale 单独扫 live 世代 TTL。

   func (e *Engine) ExpireStale(ctx context.Context, limit int, actor string) (int, error) {
	  if e == nil {
		  return 0, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	   live, err := e.gens.ListLive(ctx, limit)
	  if err != nil {
		   return 0, err
	   }
	   n := 0
	   now := e.now()
	   actor = actorOr(actor)
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  for i := range live {
		   g := live[i]
		  if !g.Expired(now) {
			   continue
		  }
		   if err := TransitionGen(&g, GenExpired); err != nil {
			   continue
		   }
		   g.ExpiredAt = now
		   if err := e.gens.Save(ctx, &g); err != nil {
			  continue
		  }
		  if half, err := e.halves.GetOpenByOutbox(ctx, g.OutboxID); err == nil && half != nil {
			   _ = TransitionHalf(half, HalfAbandoned)
			  half.AbandonedAt = now
			   _ = e.halves.Save(ctx, half)
		   }
		   e.audit(ctx, g.GenID, g.OutboxID, actor, ActionExpire, OutcomeExpired, "ttl scan", g.Generation)
		   recordExpire()
		   n++
	   }
	  return n, nil
  }
