package recon

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
)

type Capturer interface {
	Shot(ctx context.Context, userID string) (Shot, error)
}

type Platform struct {
	tickets *Book
	cap     Capturer
	led *ledger.Service
	Halt HaltEngager // 可选；漂移冻账时可 Engage，不得 Clear
}

  func NewPlatform(led *ledger.Service) *Platform {
	  var cap Capturer
	  if led != nil {
		   cap = liveShot{led: led}
	  }
	   return NewPlatformWith(led, cap)
  }

   func NewPlatformWith(led *ledger.Service, cap Capturer) *Platform {
	  return &Platform{tickets: New(), cap: cap, led: led}
  }

   func (p *Platform) Tickets() *Book { return p.tickets }

type RunReport struct {
	Compare     CompareReport `json:"compare"`
	Suggestions []Suggestion  `json:"suggestions"`
	Tickets     []Ticket      `json:"tickets"`
	Exceptions  []Exception   `json:"exceptions,omitempty"`
	Drift       DriftAction   `json:"drift"`
}

// Run 读三方余额、开差异单、按建议冻漂移。actor 空拒。
   func (p *Platform) Run(ctx context.Context, actor, userID string) (RunReport, error) {
	  if p == nil || p.led == nil {
		  return RunReport{}, ErrNotFound
	   }
	   if err := actorOK(actor); err != nil {
		  return RunReport{}, err
	  }
	   views, err := p.led.Accounts(ctx, userID)
	  if err != nil {
		  return RunReport{}, err
	   }
	  var shot Shot
	  if p.cap != nil {
		   shot, err = p.cap.Shot(ctx, userID)
		   if err != nil {
			   return RunReport{}, err
		   }
	  }
	   bills, err := p.led.ListBills(ctx, userID, 200)
	   if err != nil {
		  return RunReport{}, err
	   }
	  rep, err := stmt.Build(userID, bills, stmt.Filter{})
	  if err != nil {
		  return RunReport{}, err
	   }
	   cmp,
		   err := CompareSources(userID, views, shot, rep)
	  if err != nil {
		   return RunReport{}, err
	  }
	   sug := Suggest(cmp)
	  out := RunReport{Compare: cmp, Suggestions: sug, Exceptions: Exceptions(cmp)}
	   if !cmp.OK {
		  tks,
			   err := p.tickets.OpenFrom(actor, ToMismatches(cmp))
		  if err != nil {
			  return out, err
		   }
		   out.Tickets = tks
		   drift, err := ApplyFreezeOnDriftWith(ctx, p.led, userID, sug, DriftHooks{
			   WF:   WrapBook(p.tickets, 0),
			  Halt: p.Halt,
		  })
		   if err != nil {
			  return out, err
		   }
		   out.Drift = drift
	   }
	  return out, nil
   }

// liveShot 用当前账本科目当快照。没有独立拍照时至少一致 live，避免空源当一致。

type liveShot struct{ led *ledger.Service }

   func (l liveShot) Shot(ctx context.Context, userID string) (Shot, error) {
	  if l.led == nil {
		   return Shot{UserID: userID}, nil
	  }
	   views, err := l.led.Accounts(ctx, userID)
	   if err != nil {
		  return Shot{}, err
	  }
	   out := Shot{UserID: userID, Positions: make([]ShotPos, 0, len(views))}
	   for _, v := range views {
		  out.Positions = append(out.Positions, ShotPos{
			   Book: v.Book, Asset: v.Asset, Available: v.Available, Frozen: v.Frozen,
		  })
	  }
	  return out, nil
  }

// BlocksFreeze 未关差异工单时挡新冻账入口；供出金/划转调用方在冻前检查。
  func (p *Platform) BlocksFreeze(userID string) error {
	  if p == nil || p.tickets == nil {
		   return nil
	  }
	  wf := &Workflow{book: p.tickets}
	  return DriftBlocksFreeze(wf, userID)
  }
