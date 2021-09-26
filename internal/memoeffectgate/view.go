package memoeffectgate

import "context"

type TicketCreditView struct {
	Ticket MemoTicketView     `json:"ticket"`
	Effect *MemoEffect        `json:"effect,omitempty"`
	Fence  *TicketCreditFence `json:"fence,omitempty"`
	Proofs     []MemoHalfCreditProof `json:"proofs,omitempty"`
	FailClosed bool                  `json:"fail_closed"`
}

  func (e *Engine) InspectTicket(ctx context.Context, ticketID string) (TicketCreditView, error) {
	  if err := e.require(); err != nil {
		  return TicketCreditView{}, err
	   }
	  tk, err := e.tickets.GetMemoTicket(ctx, ticketID)
	  if err != nil {
		  return TicketCreditView{}, err
	   }
	   view := TicketCreditView{Ticket: tk}
	  if eff, err := e.store.GetEffectByTicket(ctx, ticketID); err == nil {
		   view.Effect = &eff
	  } else {
		  view.FailClosed = true
	   }
	  if fn, err := e.store.GetFenceByTicket(ctx, ticketID); err == nil {
		  view.Fence = &fn
	  }
	   if ps, err := e.store.ListProofsByTicket(ctx, ticketID, 20); err == nil {
		   view.Proofs = ps
	  }
	   if view.Effect == nil && tk.Status == "pending" {
		  view.FailClosed = true
	   }
	  return view, nil
   }

  func (e *Engine) CountOpenHalf(ctx context.Context) (int, error) {
	  if err := e.require(); err != nil {
		   return 0, err
	  }
	  rows, err := e.store.ListProofsByState(ctx, HalfOpen, 500)
	   if err != nil {
		  return 0, err
	   }
	  return len(rows), nil
  }

  func (e *Engine) CountBound(ctx context.Context) (int, error) {
	  if err := e.require(); err != nil {
		  return 0, err
	   }
	  return e.store.CountEffects(ctx,
		   EffectBound)
  }

   func (e *Engine) CountCredited(ctx context.Context) (int, error) {
	   if err := e.require(); err != nil {
		   return 0, err
	   }
	   return e.store.CountEffects(ctx, EffectCredited)
  }

   func SilentKeepInvariant(p MemoHalfCreditProof) bool {
	  return !p.SilentKeep
  }

  func ApproveSuccessInvariant(resp ApproveMemoWithEffectResp) bool {
	  if !resp.OK {
		   return false
	   }
	  if resp.Effect.ID == "" || resp.Effect.OutputKey == "" {
		  return false
	   }
	  if resp.Ticket.Status != "approved" {
		   return false
	  }
	  if resp.Fence.Phase != FenceSealed || resp.Fence.CreditCount != 1 {
		   return false
	  }
	  return true
   }

   func RejectGuardInvariant(resp DecideMemoGuardedResp, observed bool) bool {
	   if !observed {
		   return resp.OK
	  }
	   if resp.Proof == nil {
		  return false
	  }
	  if resp.Proof.SilentKeep {
		   return false
	   }
	  return resp.Blocked || resp.OK
   }
// temporary guard removed after the production check
