package memoeffectgate

import (
	"context"
	"strings"
	"time"
)

   func (e *Engine) ApproveMemoWithEffect(ctx context.Context, req ApproveMemoWithEffectReq) (ApproveMemoWithEffectResp, error) {
	  if err := e.require(); err != nil {
		  return ApproveMemoWithEffectResp{}, err
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   asset := normalizeAsset(req.Asset)
	   if asset == "" {
		   return ApproveMemoWithEffectResp{}, ErrAssetRequired
	  }
	   if strings.TrimSpace(req.TicketID) == "" {
		  return ApproveMemoWithEffectResp{}, ErrTicketRequired
	   }
	   ttl := e.ttl
	  if req.TTL != "" {
		  d, err := time.ParseDuration(req.TTL)
		   if err != nil || d <= 0 {
			   return ApproveMemoWithEffectResp{},
				  ErrTTLInvalid
		   }
		   ttl = d
	  }
	  if req.Idem != "" {
		  if prev, err := e.store.GetEffectByIdem(ctx, req.Idem); err == nil {
			  if prev.TicketID != req.TicketID {
				  return ApproveMemoWithEffectResp{}, ErrIdemConflict
			  }
			  if prev.State == EffectCredited {
				   tk, _ := e.tickets.GetMemoTicket(ctx, req.TicketID)
				  fn, _ := e.store.GetFenceByTicket(ctx, req.TicketID)
				  return ApproveMemoWithEffectResp{Ticket: tk, Effect: prev, Fence: fn, OK: true}, nil
			  }
			  if prev.State == EffectBound {
				   return e.resumeApprove(ctx,
					   prev, req.Actor)
			  }
		   }
	  }
	   if prev, err := e.store.GetEffectByTicket(ctx, req.TicketID); err == nil {
		  if prev.State == EffectCredited {
			  tk, _ := e.tickets.GetMemoTicket(ctx, req.TicketID)
			  fn, _ := e.store.GetFenceByTicket(ctx, req.TicketID)
			   return ApproveMemoWithEffectResp{Ticket: tk, Effect: prev, Fence: fn, OK: true}, nil
		  }
		  if prev.State == EffectBound {
			  return e.resumeApprove(ctx, prev, req.Actor)
		  }
	   }
	   ticket, err := e.tickets.GetMemoTicket(ctx, req.TicketID)
	  if err != nil {
		  return ApproveMemoWithEffectResp{}, err
	  }
	  if !strings.EqualFold(ticket.Status, "pending") {
		  _ = e.sealProof(ctx, MemoHalfCreditProof{
			  Kind: ProofTicketNotPending, TicketID: ticket.ID, Blocked: true,
			   Detail: "ticket status=" + ticket.Status, Actor: req.Actor,
		  })
		  return ApproveMemoWithEffectResp{Ticket: ticket},
			   ErrTicketNotPending
	  }
	  key, err := CanonicalOutputKey(ticket.Chain, ticket.TxHash, req.OutputIndex)
	   if err != nil {
		  return ApproveMemoWithEffectResp{Ticket: ticket}, err
	  }
	   if occ, ok, _ := e.mutex.Lookup(ctx, key); ok && occ.Taken && occ.Holder != HolderMemoApprove {
		   kind := ProofOutputOccupied
		  fail := ErrOutputOccupied
		  if occ.AutoClaimed {
			   fail = ErrAutoClaimOccupied
		  }
		  _ = e.sealProof(ctx, MemoHalfCreditProof{
			  Kind: kind, TicketID: ticket.ID, Chain: ticket.Chain, TxHash: ticket.TxHash,
			   OutputIndex: req.OutputIndex, Blocked: true, Sealed: true,
			  Detail: "output occupied by " + occ.Holder, Actor: req.Actor,
		  })
		  fresh,
			  _ := e.tickets.GetMemoTicket(ctx, ticket.ID)
		   return ApproveMemoWithEffectResp{Ticket: fresh}, fail
	   }
	  ok, occ, err := e.mutex.Acquire(ctx, key, HolderMemoApprove, ticket.ID)
	  if err != nil || !ok {
		   if occ.AutoClaimed {
			  return ApproveMemoWithEffectResp{Ticket: ticket}, ErrAutoClaimOccupied
		  }
		   if err == nil {
			  err = ErrAcquireFailed
		   }
		   fresh, _ := e.tickets.GetMemoTicket(ctx, ticket.ID)
		   return ApproveMemoWithEffectResp{Ticket: fresh}, err
	   }
	   now := e.now()
	   eff := MemoEffect{
		   ID: e.newID(), TicketID: ticket.ID, UserID: ticket.UserID,
		   Chain:  ticket.Chain,
		  TxHash: ticket.TxHash, OutputIndex: req.OutputIndex,
		  OutputKey: key, Asset: asset, Memo: ticket.Memo, Address: ticket.Address,
		   State: EffectBound, Holder: HolderMemoApprove, Idem: strings.TrimSpace(req.Idem),
		  TTL: Duration(ttl), BoundAt: now, ExpiresAt: now.Add(ttl),
		   Actor: req.Actor, CreatedAt: now, UpdatedAt: now,
	   }
	   if _, err := e.store.SaveEffect(ctx, eff); err != nil {
		   return ApproveMemoWithEffectResp{Ticket: ticket}, err
	  }
	   e.audit(ctx, eff.ID, EffectEvBind, "", EffectBound, req.Actor, "bind output "+key)
	  fence := TicketCreditFence{
		   ID: e.newID(), EffectID: eff.ID, TicketID: ticket.ID, Phase: FenceIdle,
		  IdemObserve: IdemObservePrefix + ticket.Chain + ":" + ticket.TxHash,
		   IdemApply: IdemApplyPrefix + ticket.ID,
		  CreatedAt: now, UpdatedAt: now,
	  }
	  if _, err := e.store.SaveFence(ctx, fence); err != nil {
		   return ApproveMemoWithEffectResp{Ticket: ticket,
			   Effect: eff}, err
	  }
	  eff.FenceID = fence.ID
	   _, _ = e.store.SaveEffect(ctx, eff)
	  return e.runFence(ctx,
		  ticket, eff, fence, req.Actor)
  }

  func (e *Engine) resumeApprove(ctx context.Context, eff MemoEffect, actor string) (ApproveMemoWithEffectResp, error) {
	   ticket, err := e.tickets.GetMemoTicket(ctx, eff.TicketID)
	   if err != nil {
		  return ApproveMemoWithEffectResp{}, err
	  }
	   if !strings.EqualFold(ticket.Status, "pending") {
		  return ApproveMemoWithEffectResp{Ticket: ticket,
			  Effect: eff}, ErrTicketNotPending
	  }
	  fence,
		   err := e.store.GetFenceByTicket(ctx, ticket.ID)
	   if err != nil {
		   return ApproveMemoWithEffectResp{Ticket: ticket, Effect: eff}, err
	  }
	  return e.runFence(ctx, ticket, eff, fence, actor)
   }

  func (e *Engine) runFence(ctx context.Context, ticket MemoTicketView, eff MemoEffect, fence TicketCreditFence, actor string) (ApproveMemoWithEffectResp, error) {
	  if strings.TrimSpace(eff.ID) == "" {
		  return ApproveMemoWithEffectResp{Ticket: ticket}, ErrEffectRequired
	  }
	  if e.now().After(eff.ExpiresAt) && eff.State == EffectBound {
		   return ApproveMemoWithEffectResp{Ticket: ticket, Effect: eff, Fence: fence}, ErrEffectExpired
	   }
	  if err := e.credit.BindMemo(ticket.UserID, ticket.Chain, ticket.Memo); err != nil {
		  fence.LastError = err.Error()
		  fence.UpdatedAt = e.now()
		   _, _ = e.store.SaveFence(ctx, fence)
		   return ApproveMemoWithEffectResp{Ticket: ticket, Effect: eff, Fence: fence}, ErrBindMemoFailed
	   }
	   if !fence.ObserveOK {
		  sight, err := e.credit.ObserveCredit(ctx, ticket.UserID, ticket.Chain, ticket.TxHash, eff.Asset, ticket.Memo, ticket.Address)
		   if err != nil {
			  fence.Phase = FenceHalfCut
			  fence.HalfCut = true
			  fence.LastError = err.Error()
			   fence.UpdatedAt = e.now()
			  _,
				  _ = e.store.SaveFence(ctx, fence)
			   p := e.sealProof(ctx, MemoHalfCreditProof{
				   Kind: ProofHalfCutObserve, EffectID: eff.ID, TicketID: ticket.ID, FenceID: fence.ID,
				   Chain: ticket.Chain, TxHash: ticket.TxHash, OutputIndex: eff.OutputIndex,
				  Blocked: true,
				   Detail:  err.Error(), Actor: actor,
			  })
			   return ApproveMemoWithEffectResp{Ticket: ticket, Effect: eff, Fence: fence, HalfProof: p}, wrapProof(ProofHalfCutObserve, ErrObserveFailed)
		  }
		  fence.ObserveOK = true
		  fence.ObservedAmt = sight.Amount
		  fence.Phase = FenceApply
		  eff.ObservedAmt = sight.Amount
		   eff.ObservedRef = sight.TransferID
		   if fence.CreditCount == 0 {
			   fence.CreditCount = 1
		  }
	  }
	   if fence.ObserveOK && !fence.ApplyOK {
		  if fence.CreditCount > 1 {
			  p := e.sealProof(ctx, MemoHalfCreditProof{
				   Kind: ProofSecondFullCredit, EffectID: eff.ID, TicketID: ticket.ID, FenceID: fence.ID,
				  Blocked: true, Sealed: true, Detail: "second full credit refused", Actor: actor,
			  })
			  return ApproveMemoWithEffectResp{Ticket: ticket, Effect: eff, Fence: fence, HalfProof: p}, ErrSecondFullCredit
		  }
		   err := e.credit.ApplyInbound(ctx, ticket.UserID, ticket.Chain, eff.Asset, fence.ObservedAmt, fence.IdemApply)
		   if err != nil {
			   fence.Phase = FenceHalfCut
			   fence.HalfCut = true
			   fence.LastError = err.Error()
			  fence.ResumeToken = fence.ID
			  fence.UpdatedAt = e.now()
			   _, _ = e.store.SaveFence(ctx, fence)
			  _, _ = e.store.SaveEffect(ctx, eff)
			  p := e.sealProof(ctx, MemoHalfCreditProof{
				   Kind: ProofHalfCutApply, EffectID: eff.ID, TicketID: ticket.ID, FenceID: fence.ID,
				   ObservedAmt: fence.ObservedAmt, Blocked: true, Detail: err.Error(), Actor: actor,
			   })
			  return ApproveMemoWithEffectResp{Ticket: ticket, Effect: eff, Fence: fence, HalfProof: p}, wrapProof(ProofHalfCutApply, ErrApplyFailed)
		  }
		  fence.ApplyOK = true
		   fence.Phase = FenceTicket
		  eff.AppliedRef = fence.IdemApply
	   }
	  if fence.ApplyOK && !fence.TicketOK {
		  advanced, err := e.tickets.AdvanceTicket(ctx, ticket.ID, "approved")
		  if err != nil {
			   fence.Phase = FenceHalfCut
			   fence.HalfCut = true
			  fence.LastError = err.Error()
			  fence.UpdatedAt = e.now()
			   _, _ = e.store.SaveFence(ctx, fence)
			  return ApproveMemoWithEffectResp{Ticket: ticket, Effect: eff, Fence: fence}, err
		  }
		   ticket = advanced
		   fence.TicketOK = true
		   now := e.now()
		  fence.Phase = FenceSealed
		   fence.HalfCut = false
		  fence.SealedAt = &now
		   fence.UpdatedAt = now
		   next, _ := MustApplyEffect(eff.State, EffectEvCredit)
		  eff.State = next
		  eff.CreditedAt = &now
		   eff.UpdatedAt = now
		  e.audit(ctx, eff.ID, EffectEvCredit, EffectBound, next, actor, "fence sealed")
	  }
	   _, _ = e.store.SaveFence(ctx, fence)
	  _, _ = e.store.SaveEffect(ctx, eff)
	   return ApproveMemoWithEffectResp{Ticket: ticket,
		  Effect: eff, Fence: fence, OK: fence.Phase == FenceSealed}, nil
   }

   func (e *Engine) sealProof(ctx context.Context, p MemoHalfCreditProof) *MemoHalfCreditProof {
	  if p.ID == "" {
		  p.ID = e.newID()
	   }
	  if p.CreatedAt.IsZero() {
		  p.CreatedAt = e.now()
	   }
	  if p.State == "" {
		  p.State = HalfOpen
	  }
	  p.SilentKeep = false
	  saved, err := e.store.SaveProof(ctx, p)
	  if err != nil {
		   return &p
	   }
	  return &saved
  }
