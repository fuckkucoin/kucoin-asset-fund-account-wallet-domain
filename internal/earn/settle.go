package earn

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Claim struct {
	PositionID string `json:"position_id"`
	Amount string `json:"amount"`
	At time.Time `json:"at"`
}

// ClaimAccrued 把未领利息记到 claimed。不写用户可用，避免从本金里“长”出钱。
// 实际入账必须另走账本 hop / 桥利息，本方法只记账务应付。
   func ClaimAccrued(p Position, now time.Time) (Position, Claim, error) {
	   if p.Status == StatusClosed {
		   return p, Claim{}, ErrClosed
	  }
	   un, err := p.Unclaimed()
	  if err != nil {
		   return p, Claim{}, err
	   }
	  if !un.IsPositive() {
		  return p, Claim{PositionID: p.ID, Amount: "0", At: now}, nil
	  }
	   claimed, err := p.ClaimedAmt()
	  if err != nil {
		   return p, Claim{}, err
	   }
	  next, err := claimed.Add(un)
	   if err != nil {
		  return p, Claim{}, err
	  }
	   p.Claimed = next.String()
	  if now.IsZero() {
		   now = time.Now().UTC()
	  }
	   return p, Claim{PositionID: p.ID, Amount: un.String(), At: now.UTC()}, nil
   }

// RedeemCheck 锁定期未满拒。本金必须仍能过 Parse。

   func RedeemCheck(p Position, now time.Time) error {
	   if p.Status == StatusRedeemed || p.Status == StatusClosed {
		   return ErrClosed
	   }
	  if p.Locked(now) {
		  return ErrLocked
	  }
	  if _, err := p.PrincipalAmt(); err != nil {
		  return err
	  }
	  return nil
   }

  func InterestLines(asset, amt string) ([]money.Line, error) {
	  a, err := money.ParseBook(orZero(amt))
	  if err != nil {
		   return nil, err
	  }
	   if !a.IsPositive() {
		  return nil, fmt.Errorf("%w: interest", money.ErrNotPositive)
	   }
	  lines := money.InterestPair(asset,
		  a)
	   if err := money.Balanced(lines); err != nil {
		   return nil, err
	   }
	  return lines, nil
  }
