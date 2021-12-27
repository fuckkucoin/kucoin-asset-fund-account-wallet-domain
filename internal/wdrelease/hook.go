package wdrelease

import "context"

// DeskView 释放台只读+受控回写视图。
// 允许写入：Ticket.State / Grant.State / LedgerTag（仅经 Desk 方法）。
// 禁止写入：金额公式、审批 Decision、补偿回滚腿。
type DeskView interface {
	LookupTicket(ctx context.Context, id string) (OccLiveTicket, bool)
	LookupGrantByIdem(ctx context.Context, idem string) (OccLiveGrant, bool)
}

// DeskAdapter 将 Desk 适配为 DeskView。
type DeskAdapter struct {
	Desk *OccDesk
}

  func (a DeskAdapter) LookupTicket(ctx context.Context, id string) (OccLiveTicket, bool) {
	  if a.Desk == nil {
		  return OccLiveTicket{}, false
	  }
	   t, err := a.Desk.GetTicket(ctx, id)
	  if err != nil || t == nil {
		   return OccLiveTicket{}, false
	  }
	   return OccLiveTicket{
		  ID: t.ID, UserID: t.UserID, Asset: t.Asset, Amount: t.Amount,
		  State: string(t.State), Epoch: t.Epoch, GrantIdem: t.GrantIdem,
	   }, true
  }

   func (a DeskAdapter) LookupGrantByIdem(ctx context.Context, idem string) (OccLiveGrant, bool) {
	   if a.Desk == nil {
		   return OccLiveGrant{}, false
	  }
	   g, err := a.Desk.GetGrantByIdem(ctx, idem)
	   if err != nil || g == nil {
		  return OccLiveGrant{}, false
	  }
	  return OccLiveGrant{
		   ID: g.ID, TicketID: g.TicketID, GrantIdem: g.GrantIdem,
		   Asset: g.Asset, Amount: g.Amount, State: string(g.State),
		   Epoch: g.Epoch,
		  LedgerTag: g.LedgerTag,
	  }, true
   }

// NopView 空视图。

type NopView struct{}

  func (NopView) LookupTicket(context.Context, string) (OccLiveTicket, bool) {
	  return OccLiveTicket{}, false
   }
   func (NopView) LookupGrantByIdem(context.Context, string) (OccLiveGrant, bool) {
	  return OccLiveGrant{}, false
   }
