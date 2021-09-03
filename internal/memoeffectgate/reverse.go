package memoeffectgate

import (
	"context"
	"strings"
)

   func (e *Engine) OpsReverseGate(ctx context.Context, req ReverseGateReq) (MemoHalfCreditProof, error) {
	   if err := e.require(); err != nil {
		  return MemoHalfCreditProof{}, err
	  }
	   if strings.TrimSpace(req.Actor) == "" {
		   return MemoHalfCreditProof{}, ErrActorRequired
	   }
	   p, err := e.store.GetProof(ctx, req.ProofID)
	   if err != nil {
		   return MemoHalfCreditProof{}, err
	   }
	  if p.State != HalfOpen {
		  return p, ErrProofNotOpen
	  }
	  now := e.now()
	  if req.AckOnly {
		   next, err := MustApplyHalf(p.State, HalfEvAck)
		  if err != nil {
			   return p, err
		   }
		   p.State = next
		  p.AckedAt = &now
		  p.Actor = req.Actor
		   if req.Reason != "" {
			   p.Detail = req.Reason
		   }
		   return e.store.SaveProof(ctx, p)
	  }
	  ticket,
		  err := e.tickets.GetMemoTicket(ctx, p.TicketID)
	  if err != nil {
		  return p, err
	   }
	  asset := "USDT"
	   if eff, eerr := e.store.GetEffect(ctx, p.EffectID); eerr == nil && eff.Asset != "" {
		  asset = eff.Asset
	  }
	   if err := e.credit.ReverseInbound(ctx, ticket.UserID, asset, p.ObservedAmt, "ops-reverse:"+p.ID); err != nil {
		   return p, err
	   }
	   next, err := MustApplyHalf(p.State, HalfEvReverse)
	  if err != nil {
		  return p, err
	   }
	  p.State = next
	   p.ReversedAt = &now
	  p.ReverseRef = "ops-reverse:" + p.ID
	   p.Actor = req.Actor
	  p.Kind = ProofReverseGate
	   return e.store.SaveProof(ctx, p)
   }
