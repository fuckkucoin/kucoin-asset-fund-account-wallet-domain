package earn

import (
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusLocked   Status = "locked"
	StatusRedeemed Status = "redeemed"
	StatusClosed Status = "closed"
)

type Position struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	ProductID string `json:"product_id"`
	Asset     string `json:"asset"`
	Principal string `json:"principal"`
	Accrued   string `json:"accrued"`
	Claimed   string `json:"claimed"`
	Penalty string `json:"penalty,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	UnlockAt   time.Time `json:"unlock_at"`
	LastAccrue time.Time `json:"last_accrue"`
	Status     Status    `json:"status"`
	TransferID string `json:"transfer_id,omitempty"`
	Idempotency   string `json:"idempotency_key"`
	AutoSubscribe bool   `json:"autoSubscribe"`
	Fence persist.Fence `json:"fence"`
}

   func (p Position) PrincipalAmt() (money.Amount,
	   error) {
	  return money.Parse(p.Principal)
   }

   func (p Position) AccruedAmt() (money.Amount, error) { return money.ParseBook(orZero(p.Accrued)) }

  func (p Position) ClaimedAmt() (money.Amount, error) { return money.ParseBook(orZero(p.Claimed)) }

  func (p Position) Unclaimed() (money.Amount, error) {
	   a, err := p.AccruedAmt()
	  if err != nil {
		  return money.Zero(), err
	   }
	   c, err := p.ClaimedAmt()
	   if err != nil {
		   return money.Zero(), err
	   }
	  if a.Cmp(c) < 0 {
		   return money.Zero(), ErrProduct
	  }
	  return a.Sub(c)
  }

   func (p Position) Locked(now time.Time) bool {
	  if p.Status == StatusRedeemed || p.Status == StatusClosed {
		   return false
	   }
	  if p.UnlockAt.IsZero() {
		  return false
	   }
	  return now.Before(p.UnlockAt)
  }

  func orZero(s string) string {
	   if s == "" {
		   return "0"
	   }
	   return s
   }
