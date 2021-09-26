package memoeffectgate

import (
	"context"
	"strings"
)

  func (e *Engine) DecideMemoGuarded(ctx context.Context, req DecideMemoGuardedReq) (DecideMemoGuardedResp, error) {
	  if err := e.require(); err != nil {
		   return DecideMemoGuardedResp{}, err
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   if strings.TrimSpace(req.Actor) == "" {
		  return DecideMemoGuardedResp{}, ErrActorRequired
	  }
	   status := normalizeStatus(req.Status)
	   if status != "approved" && status != "rejected" {
		  return DecideMemoGuardedResp{}, ErrStatusInvalid
	  }
	  ticket, err := e.tickets.GetMemoTicket(ctx, req.TicketID)
	   if err != nil {
		   return DecideMemoGuardedResp{}, err
	   }
	   if status == "approved" {
		   return DecideMemoGuardedResp{Ticket: ticket, Blocked: true}, ErrStatusInvalid
	   }
	   eff, effErr := e.store.GetEffectByTicket(ctx, ticket.ID)
	  fence, fenceErr := e.store.GetFenceByTicket(ctx, ticket.ID)
	  observed := fenceErr == nil && fence.ObserveOK
	   if !observed && effErr == nil && strings.TrimSpace(eff.ObservedAmt) != "" {
		  observed = true
	   }
	   if observed {
		  if !req.ForceReverse && !req.Quarantine {
			  p := e.sealProof(ctx, MemoHalfCreditProof{
				   Kind:     ProofRejectAfterCredit,
				  EffectID: eff.ID, TicketID: ticket.ID,
				  FenceID: fence.ID, Chain: ticket.Chain, TxHash: ticket.TxHash,
				   OutputIndex: eff.OutputIndex, ObservedAmt: firstNonEmpty(fence.ObservedAmt, eff.ObservedAmt),
				   Blocked: true, Sealed: true, SilentKeep: false,
				  Detail: "reject after credit requires reverse or quarantine", Actor: req.Actor,
			   })
			  if effErr == nil && CanRejectBlock(eff.State) {
				  next, _ := MustApplyEffect(eff.State, EffectEvReject)
				   eff.State = next
				  now := e.now()
				   eff.RejectedAt = &now
				  eff.ProofID = p.ID
				   eff.UpdatedAt = now
				   _, _ = e.store.SaveEffect(ctx, eff)
				  e.audit(ctx, eff.ID, EffectEvReject, EffectBound, next, req.Actor, "reject after credit blocked")
			  }
			   return DecideMemoGuardedResp{Ticket: ticket, Effect: &eff, Proof: p, Blocked: true}, ErrRejectAfterCredit
		   }
		   p := e.sealProof(ctx, MemoHalfCreditProof{
			   Kind: ProofRejectAfterCredit, EffectID: eff.ID, TicketID: ticket.ID,
			  FenceID: fence.ID, Chain: ticket.Chain, TxHash: ticket.TxHash,
			  OutputIndex: eff.OutputIndex, ObservedAmt: firstNonEmpty(fence.ObservedAmt, eff.ObservedAmt),
			   Blocked: false, Sealed: true, SilentKeep: false,
			  Detail: req.Reason, Actor: req.Actor,
		  })
		  if req.ForceReverse {
			   revErr := e.credit.ReverseInbound(ctx, ticket.UserID, firstNonEmpty(eff.Asset, "USDT"), p.ObservedAmt, "memo-reverse:"+ticket.ID)
			  if revErr != nil {
				   return DecideMemoGuardedResp{Ticket: ticket,
					   Effect: &eff, Proof: p, Blocked: true}, revErr
			  }
			  now := e.now()
			  next, err := MustApplyHalf(p.State, HalfEvReverse)
			  if err != nil {
				   return DecideMemoGuardedResp{Ticket: ticket, Proof: p}, err
			  }
			   p.State = next
			  p.ReversedAt = &now
			  p.ReverseRef = "memo-reverse:" + ticket.ID
			   saved,
				   _ := e.store.SaveProof(ctx, *p)
			  p = &saved
		   }
		   if req.Quarantine {
			   if e.reverse != nil {
				   if err := e.reverse.Quarantine(ctx, ticket.UserID, firstNonEmpty(eff.Asset, "USDT"), p.ObservedAmt, req.Reason); err != nil {
					  return DecideMemoGuardedResp{Ticket: ticket, Proof: p, Blocked: true}, err
				  }
			   }
			   if effErr == nil && CanRejectBlock(eff.State) {
				   next, _ := MustApplyEffect(eff.State, EffectEvQuarantine)
				   eff.State = next
				   now := e.now()
				  eff.UpdatedAt = now
				  _, _ = e.store.SaveEffect(ctx, eff)
			  }
			  p.Kind = ProofQuarantine
			  saved, _ := e.store.SaveProof(ctx, *p)
			  p = &saved
		  }
		  advanced, err := e.tickets.AdvanceTicket(ctx, ticket.ID, "rejected")
		  if err != nil {
			  return DecideMemoGuardedResp{Ticket: ticket, Effect: &eff, Proof: p, Blocked: true}, err
		   }
		   if !req.Quarantine && effErr == nil && CanRejectBlock(eff.State) {
			  next, _ := MustApplyEffect(eff.State, EffectEvReject)
			   eff.State = next
			  now := e.now()
			  eff.RejectedAt = &now
			   eff.UpdatedAt = now
			  _, _ = e.store.SaveEffect(ctx, eff)
		   }
		  return DecideMemoGuardedResp{Ticket: advanced, Effect: &eff, Proof: p, OK: true}, nil
	  }
	  advanced, err := e.tickets.AdvanceTicket(ctx, ticket.ID, "rejected")
	  if err != nil {
		   return DecideMemoGuardedResp{Ticket: ticket}, err
	  }
	  if effErr == nil && CanRejectBlock(eff.State) {
		   next,
			  _ := MustApplyEffect(eff.State, EffectEvReject)
		  eff.State = next
		   now := e.now()
		   eff.RejectedAt = &now
		  eff.UpdatedAt = now
		   _, _ = e.store.SaveEffect(ctx, eff)
		  return DecideMemoGuardedResp{Ticket: advanced,
			   Effect: &eff, OK: true}, nil
	   }
	  return DecideMemoGuardedResp{Ticket: advanced, OK: true}, nil
  }

   func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return a
	  }
	   return b
   }
