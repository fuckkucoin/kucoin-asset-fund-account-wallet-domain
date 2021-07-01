package fiatsettlegate

import (
	"context"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/fiatrail"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// RailReader 法币轨道只读视图。
type RailReader interface {
	Get(id string) (fiatrail.Booking, error)
	KYC() *fiatrail.Gate
	Limits() *fiatrail.Limits
	PSP() *fiatrail.PSP
}

// RailBinder 回写租约号与终态，不改 Booking 图定义。

type RailBinder interface {
	Authorize(id string) (fiatrail.Booking, error)
	BindSettleLease(id, leaseID, reservationID string) (fiatrail.Booking, error)
	BindLimitsReservation(id,
		reservationID string) (fiatrail.Booking, error)
	ApplySettled(id, ledgerID, actor, leaseID string) (fiatrail.Booking, error)
	ApplyRefunded(id, refundLedger, actor, reason string) (fiatrail.Booking, error)
	Book(req fiatrail.BookReq) (fiatrail.Booking,
		error)
	ScreenKYC(id string) (fiatrail.Booking, error)
}

// LedgerHook 只读消费 Create 契约；由闸在 Settle 成功路径调用一次。
type LedgerHook interface {
	Create(ctx context.Context, req ledger.CreateReq) (*ledger.Transfer, error)
}

// PSPHook Capture / Void / Refund。闸保证 Capture 每 Intent 至多一次成功。
type PSPHook interface {
	Capture(id string) (fiatrail.Intent, error)
	Void(id string) (fiatrail.Intent, error)
	Refund(id string) (fiatrail.Intent, error)
	Get(id string) (fiatrail.Intent, error)
}

// LimitsHook Check / Record / Fingerprint。
type LimitsHook interface {
	Check(userID, amount, card string, kind fiatrail.Kind) error
	Record(userID, amount string, kind fiatrail.Kind)
	Fingerprint() string
}

// KYCHook 档位复验。

type KYCHook interface {
	Check(userID string, kind fiatrail.Kind, amount string) (fiatrail.Profile, error)
	KYCFingerprint(userID string) string
}

// RailBundle 将 fiatrail.Service 适配为挂钩。
type RailBundle struct {
	Rail *fiatrail.Service
	Led  LedgerHook
}

   func (b RailBundle) Get(id string) (fiatrail.Booking, error) {
	  if b.Rail == nil {
		   return fiatrail.Booking{}, ErrBooking
	   }
	   return b.Rail.Get(id)
   }

   func (b RailBundle) KYC() *fiatrail.Gate {
	  if b.Rail == nil {
		  return fiatrail.NewGate()
	   }
	  return b.Rail.KYC()
  }

   func (b RailBundle) Limits() *fiatrail.Limits {
	  if b.Rail == nil {
		   return fiatrail.NewLimits()
	   }
	   return b.Rail.Limits()
   }

  func (b RailBundle) PSP() *fiatrail.PSP {
	  if b.Rail == nil {
		   return fiatrail.NewPSP()
	   }
	  return b.Rail.PSP()
   }

  func (b RailBundle) Authorize(id string) (fiatrail.Booking, error) {
	  if b.Rail == nil {
		  return fiatrail.Booking{}, ErrUnavailable
	   }
	   return b.Rail.Authorize(id)
   }

   func (b RailBundle) BindSettleLease(id, leaseID, reservationID string) (fiatrail.Booking, error) {
	   if b.Rail == nil {
		   return fiatrail.Booking{}, ErrUnavailable
	   }
	  return b.Rail.BindSettleLease(id, leaseID, reservationID)
   }

   func (b RailBundle) BindLimitsReservation(id, reservationID string) (fiatrail.Booking, error) {
	  if b.Rail == nil {
		   return fiatrail.Booking{}, ErrUnavailable
	   }
	   return b.Rail.BindLimitsReservation(id, reservationID)
   }

   func (b RailBundle) ApplySettled(id, ledgerID, actor, leaseID string) (fiatrail.Booking, error) {
	  if b.Rail == nil {
		  return fiatrail.Booking{}, ErrUnavailable
	   }
	  return b.Rail.ApplySettled(id, ledgerID, actor, leaseID)
   }

   func (b RailBundle) ApplyRefunded(id, refundLedger, actor, reason string) (fiatrail.Booking, error) {
	  if b.Rail == nil {
		  return fiatrail.Booking{}, ErrUnavailable
	  }
	  return b.Rail.ApplyRefunded(id, refundLedger, actor, reason)
  }

  func (b RailBundle) Book(req fiatrail.BookReq) (fiatrail.Booking, error) {
	  if b.Rail == nil {
		   return fiatrail.Booking{}, ErrUnavailable
	  }
	   return b.Rail.Book(req)
  }

  func (b RailBundle) ScreenKYC(id string) (fiatrail.Booking, error) {
	   if b.Rail == nil {
		  return fiatrail.Booking{}, ErrUnavailable
	  }
	  return b.Rail.ScreenKYC(id)
  }

   func (b RailBundle) Create(ctx context.Context, req ledger.CreateReq) (*ledger.Transfer, error) {
	   if b.Led == nil {
		  return nil, ErrLedger
	   }
	  return b.Led.Create(ctx, req)
  }

  func (b RailBundle) Capture(id string) (fiatrail.Intent, error) {
	  return b.PSP().Capture(id)
  }

   func (b RailBundle) Void(id string) (fiatrail.Intent, error) {
	   return b.PSP().Void(id)
  }

   func (b RailBundle) Refund(id string) (fiatrail.Intent, error) {
	   return b.PSP().Refund(id)
  }

  func (b RailBundle) IntentGet(id string) (fiatrail.Intent, error) {
	   return b.PSP().Get(id)
   }

  func (b RailBundle) CheckLimits(userID, amount, card string, kind fiatrail.Kind) error {
	  return b.Limits().Check(userID, amount, card, kind)
  }

   func (b RailBundle) RecordLimits(userID, amount string, kind fiatrail.Kind) {
	  b.Limits().Record(userID, amount, kind)
  }

   func (b RailBundle) LimitsFP() string {
	  return b.Limits().Fingerprint()
  }

  func (b RailBundle) CheckKYC(userID string, kind fiatrail.Kind, amount string) (fiatrail.Profile, error) {
	  return b.KYC().Check(userID,
		  kind, amount)
  }

   func (b RailBundle) KYCFP(userID string) string {
	  return b.KYC().KYCFingerprint(userID)
   }

// LedgerPost 按预约方向入账。幂等键绑定 booking，禁止双花。
   func LedgerPost(ctx context.Context, led LedgerHook, b fiatrail.Booking, suffix string) (*ledger.Transfer, error) {
	   if led == nil {
		   return nil, ErrLedger
	   }
	  amt, err := decimal.NewFromString(strings.TrimSpace(b.Amount))
	   if err != nil {
		  return nil, ErrLedger
	   }
	  dir := ledger.FiatOnramp
	  if b.Kind == fiatrail.KindOfframp {
		   dir = ledger.FiatOfframp
	  }
	  idem := "fiat-settle:" + b.ID
	   if suffix != "" {
		  idem = idem + ":" + suffix
	   }
	   return led.Create(ctx, ledger.CreateReq{
		  UserID: b.UserID, Direction: dir, Asset: b.Asset, Amount: amt,
		   IdempotencyKey: idem,
		  WaitConfirm:    true,
	   })
   }

// ReverseKind 退款反方向。
  func ReverseKind(k fiatrail.Kind) fiatrail.Kind {
	   if k == fiatrail.KindOnramp {
		  return fiatrail.KindOfframp
	  }
	  return fiatrail.KindOnramp
  }
