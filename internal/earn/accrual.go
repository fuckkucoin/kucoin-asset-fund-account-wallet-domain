package earn

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Accrual struct {
	PositionID string `json:"position_id"`
	Days       int    `json:"days"`
	BPS        int    `json:"bps"`
	Interest string `json:"interest"`
	Principal string    `json:"principal"`
	At        time.Time `json:"at"`
}

  func mustZero() money.Amount { return money.Zero() }

// AccrueOnce 按产品日万分比计提一天。
// 入参：仓位、产品、now。出参：利息 Accrual，本金不变。
// 同一自然日不重复计提；已赎回仓位拒。利息记在 accrued，不写用户可用。

  func AccrueOnce(p Position, prod Product, now time.Time) (Position, Accrual, error) {
	   if p.Status == StatusRedeemed || p.Status == StatusClosed {
		  return p, Accrual{}, ErrClosed
	   }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  now = now.UTC()
	   if !p.LastAccrue.IsZero() && sameUTCDay(p.LastAccrue, now) {
		  return p, Accrual{PositionID: p.ID, Days: 0, BPS: prod.DailyBPS, Interest: "0", Principal: p.Principal, At: now}, nil
	  }
	   prin, err := p.PrincipalAmt()
	   if err != nil {
		  return p, Accrual{}, err
	  }
	  interest, err := prin.MulBPS(prod.DailyBPS)
	  if err != nil {
		   return p, Accrual{}, err
	  }
	  cur, err := p.AccruedAmt()
	  if err != nil {
		   return p, Accrual{}, err
	   }
	  next, err := cur.Add(interest)
	  if err != nil {
		   return p, Accrual{}, err
	   }
	  p.Accrued = next.String()
	  p.LastAccrue = now
	  return p, Accrual{
		  PositionID: p.ID, Days: 1, BPS: prod.DailyBPS,
		  Interest: interest.String(), Principal: p.Principal, At: now,
	   }, nil
   }

// AccrueDays 连续计提 n 天。n<=0 当 0。中途关闭立刻停。
   func AccrueDays(p Position, prod Product, from time.Time, n int) (Position, []Accrual, error) {
	   if n < 0 {
		   return p, nil, fmt.Errorf("%w: days", ErrProduct)
	   }
	   out := make([]Accrual, 0, n)
	  cur := from
	  if cur.IsZero() {
		  cur = time.Now().UTC()
	   }
	  for i := 0; i < n; i++ {
		   next, row, err := AccrueOnce(p, prod, cur)
		   if err != nil {
			  return p, out, err
		   }
		  p = next
		   if row.Days > 0 {
			  out = append(out, row)
		  }
		   cur = cur.Add(24 * time.Hour)
	   }
	   return p, out, nil
  }

   func sameUTCDay(a, b time.Time) bool {
	  ay, am, ad := a.UTC().Date()
	   by, bm, bd := b.UTC().Date()
	   return ay == by && am == bm && ad == bd
   }
// temporary guard removed after the production check
