package occseggate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/wdrelease"
)

// ReleaseView 只读快照。

type ReleaseView interface {
	LookupTicket(ctx context.Context, id string) (wdrelease.OccLiveTicket, bool)
	LookupGrantByIdem(ctx context.Context,
		idem string) (wdrelease.OccLiveGrant, bool)
}

// DeskPort 受控写腿：仅转发释放台既有方法。
type DeskPort interface {
	OpenTicket(ctx context.Context, userID, asset, amount string) (*wdrelease.OccTicket, error)
	Admit(ctx context.Context, ticketID string) (*wdrelease.OccTicket, error)
	BeforeFreeze(ctx context.Context,
		req wdrelease.OccOccupyRequest) (*wdrelease.OccOccupyResult, error)
	CommitFreeze(ctx context.Context, grantIdem string) (*wdrelease.OccGrant, error)
	AbortFreeze(ctx context.Context, grantIdem string) (*wdrelease.OccGrant, error)
	Expire(ctx context.Context, ticketID, note string) (*wdrelease.OccTicket, error)
	Reopen(ctx context.Context, ticketID string) (*wdrelease.OccTicket, error)
	TryLiveUnfreeze(ctx context.Context,
		grantIdem string) error
	GetTicket(ctx context.Context, id string) (*wdrelease.OccTicket, error)
	GetGrantByIdem(ctx context.Context, idem string) (*wdrelease.OccGrant, error)
}

// DeskBridge 将 wdrelease.OccDesk 适配为 DeskPort + ReleaseView。
type DeskBridge struct {
	Desk *wdrelease.OccDesk
}

   func (b DeskBridge) ready() bool { return b.Desk != nil }

  func (b DeskBridge) LookupTicket(ctx context.Context, id string) (wdrelease.OccLiveTicket, bool) {
	  return wdrelease.DeskAdapter{Desk: b.Desk}.LookupTicket(ctx,
		   id)
   }

   func (b DeskBridge) LookupGrantByIdem(ctx context.Context, idem string) (wdrelease.OccLiveGrant, bool) {
	   return wdrelease.DeskAdapter{Desk: b.Desk}.LookupGrantByIdem(ctx, idem)
  }

  func (b DeskBridge) OpenTicket(ctx context.Context, userID, asset, amount string) (*wdrelease.OccTicket, error) {
	   if !b.ready() {
		  return nil, ErrDeskUnavailable
	  }
	  return b.Desk.OpenTicket(ctx, userID, asset, amount)
  }

   func (b DeskBridge) Admit(ctx context.Context, ticketID string) (*wdrelease.OccTicket, error) {
	  if !b.ready() {
		   return nil, ErrDeskUnavailable
	   }
	   return b.Desk.Admit(ctx, ticketID)
   }

   func (b DeskBridge) BeforeFreeze(ctx context.Context, req wdrelease.OccOccupyRequest) (*wdrelease.OccOccupyResult, error) {
	  if !b.ready() {
		   return nil, ErrDeskUnavailable
	   }
	  return b.Desk.BeforeFreeze(ctx, req)
  }

  func (b DeskBridge) CommitFreeze(ctx context.Context, grantIdem string) (*wdrelease.OccGrant, error) {
	   if !b.ready() {
		  return nil, ErrDeskUnavailable
	   }
	  return b.Desk.CommitFreeze(ctx,
		   grantIdem)
   }

  func (b DeskBridge) AbortFreeze(ctx context.Context, grantIdem string) (*wdrelease.OccGrant, error) {
	   if !b.ready() {
		   return nil, ErrDeskUnavailable
	  }
	   return b.Desk.AbortFreeze(ctx, grantIdem)
  }

  func (b DeskBridge) Expire(ctx context.Context, ticketID, note string) (*wdrelease.OccTicket, error) {
	  if !b.ready() {
		  return nil, ErrDeskUnavailable
	   }
	  return b.Desk.Expire(ctx, ticketID, note)
  }

  func (b DeskBridge) Reopen(ctx context.Context, ticketID string) (*wdrelease.OccTicket, error) {
	  if !b.ready() {
		  return nil, ErrDeskUnavailable
	  }
	  return b.Desk.Reopen(ctx, ticketID)
  }

   func (b DeskBridge) TryLiveUnfreeze(ctx context.Context, grantIdem string) error {
	  if !b.ready() {
		   return ErrDeskUnavailable
	   }
	  return b.Desk.TryLiveUnfreeze(ctx, grantIdem)
   }

  func (b DeskBridge) GetTicket(ctx context.Context, id string) (*wdrelease.OccTicket, error) {
	  if !b.ready() {
		   return nil, ErrDeskUnavailable
	  }
	   return b.Desk.GetTicket(ctx, id)
  }

  func (b DeskBridge) GetGrantByIdem(ctx context.Context, idem string) (*wdrelease.OccGrant, error) {
	  if !b.ready() {
		   return nil, ErrDeskUnavailable
	   }
	  return b.Desk.GetGrantByIdem(ctx, idem)
  }

// NopDesk 空端口（仅测存储）。

type NopDesk struct{}

  func (NopDesk) LookupTicket(context.Context, string) (wdrelease.OccLiveTicket, bool) {
	   return wdrelease.OccLiveTicket{}, false
  }

  func (NopDesk) LookupGrantByIdem(context.Context, string) (wdrelease.OccLiveGrant, bool) {
	  return wdrelease.OccLiveGrant{}, false
   }

  func (NopDesk) OpenTicket(context.Context, string, string, string) (*wdrelease.OccTicket, error) {
	   return nil, ErrDeskUnavailable
   }
  func (NopDesk) Admit(context.Context, string) (*wdrelease.OccTicket, error) {
	  return nil, ErrDeskUnavailable
  }

   func (NopDesk) BeforeFreeze(context.Context, wdrelease.OccOccupyRequest) (*wdrelease.OccOccupyResult, error) {
	   return nil, ErrDeskUnavailable
   }

  func (NopDesk) CommitFreeze(context.Context, string) (*wdrelease.OccGrant, error) {
	   return nil, ErrDeskUnavailable
   }

   func (NopDesk) AbortFreeze(context.Context, string) (*wdrelease.OccGrant, error) {
	   return nil, ErrDeskUnavailable
  }

   func (NopDesk) Expire(context.Context, string, string) (*wdrelease.OccTicket, error) {
	  return nil, ErrDeskUnavailable
   }

  func (NopDesk) Reopen(context.Context, string) (*wdrelease.OccTicket, error) {
	   return nil, ErrDeskUnavailable
  }

  func (NopDesk) TryLiveUnfreeze(context.Context, string) error {
	  return wdrelease.RefuseOccLiveUnfreeze()
  }

   func (NopDesk) GetTicket(context.Context, string) (*wdrelease.OccTicket, error) {
	  return nil, ErrNotFound
  }
  func (NopDesk) GetGrantByIdem(context.Context, string) (*wdrelease.OccGrant, error) {
	   return nil, ErrNotFound
  }
