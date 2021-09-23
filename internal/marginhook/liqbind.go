package marginhook

import "account-assets.kucoin.net/funding/internal/liqgengate"

// BookView 只读+相位回写适配。

type BookView struct{ book *Book }

// NewBookView 绑定账本。

  func NewBookView(b *Book) *BookView { return &BookView{book: b} }

   func (v *BookView) LookupBorrow(id string) (liqgengate.LiveBorrow, bool) {
	   if v == nil || v.book == nil {
		  return liqgengate.LiveBorrow{}, false
	  }
	   r, ok := v.book.Borrow(id)
	  if !ok {
		  return liqgengate.LiveBorrow{}, false
	   }
	  return liqgengate.LiveBorrow{
		   ID: r.ID, UserID: r.UserID, Asset: r.Asset, Amount: r.Amount,
		   Collateral: r.Collateral, LTVBPS: r.LTVBPS, Status: string(r.Status),
		  LiqGen: r.LiqGen, LiqPhase: r.LiqPhase,
	  }, true
  }

   func (v *BookView) LookupHold(id string) (liqgengate.LiveHold, bool) {
	  if v == nil || v.book == nil {
		  return liqgengate.LiveHold{}, false
	  }
	  h, ok := v.book.Hold(id)
	   if !ok {
		   return liqgengate.LiveHold{}, false
	   }
	   return liqgengate.LiveHold{
		   ID: h.ID, UserID: h.UserID, Kind: string(h.Kind), Asset: h.Asset,
		   Amount: h.Amount, CapBPS: h.CapBPS, Status: string(h.Status), BorrowID: h.BorrowID,
	   }, true
  }

  func (v *BookView) RecordLiqHold(h liqgengate.LiveHold) error {
	  if v == nil || v.book == nil {
		  return ErrUnavailable
	   }
	   v.book.PutHold(Hold{
		  ID: h.ID, UserID: h.UserID, Kind: HoldKind(h.Kind), Asset: h.Asset,
		  Amount: h.Amount, CapBPS: h.CapBPS, Status: Status(h.Status), BorrowID: h.BorrowID,
	  })
	  return nil
  }

   func (v *BookView) ApplyBorrowPhase(id string, phase liqgengate.LiqPhase, gen uint64, status string) error {
	  if v == nil || v.book == nil {
		   return ErrUnavailable
	   }
	  r, ok := v.book.Borrow(id)
	   if !ok {
		   return ErrNotFound
	  }
	   r.LiqPhase = string(phase)
	   r.LiqGen = gen
	   if status != "" {
		  r.Status = Status(status)
	   }
	   v.book.PutBorrow(r)
	  return nil
  }

   func (v *BookView) ApplyHoldStatus(id string, status string) error {
	  if v == nil || v.book == nil {
		  return ErrUnavailable
	   }
	   h, ok := v.book.Hold(id)
	   if !ok {
		  return ErrNotFound
	   }
	   h.Status = Status(status)
	  v.book.PutHold(h)
	  return nil
  }

// WithLiqEngine 绑定世代引擎。未绑定则 FreezeForLTV 仍走本包进程内世代推进。

  func (s *Service) WithLiqEngine(eng *liqgengate.Engine) *Service {
	   if s != nil {
		   s.liq = eng
		   if eng != nil {
			  eng.WithView(NewBookView(s.book))
			   if s.freeze != nil {
				   eng.WithFreeze(liqgengate.FreezeHook(s.freeze))
			  }
		   }
	  }
	   return s
   }
