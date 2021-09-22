package p2pgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

// CoolingView 卖出冷却读。
type CoolingView struct {
	UserID string `json:"user_id"`
	Locked bool   `json:"locked"`
	Until   string `json:"until,omitempty"`
	UntilMs int64  `json:"until_ms,omitempty"`
	RemainingS  int64  `json:"remaining_s,omitempty"`
	SellBlocked bool   `json:"sell_blocked"`
	Note        string `json:"note,omitempty"`
}

// CoolingState 读出金冷却。查询失败返回 Locked=true。

   func (f *Funds) CoolingState(ctx context.Context, userID string) (CoolingView, error) {
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return CoolingView{}, err
	   }
	  view := CoolingView{UserID: uid, Note: "sell hold reuses withdraw cooling"}
	  if f == nil || f.led == nil {
		  view.Locked = true
		  view.SellBlocked = true
		   view.Note = "ledger unavailable; fail-closed"
		  return view, nil
	   }
	   until, _, err := f.led.WithdrawLockUntil(ctx, uid)
	   if err != nil {
		   view.Locked = true
		  view.SellBlocked = true
		  view.Note = "cooling query failed; fail-closed"
		   return view, nil
	   }
	   now := time.Now().UTC()
	   if !until.IsZero() && until.After(now) {
		  view.Locked = true
		  view.SellBlocked = true
		   view.Until = until.Format(time.RFC3339)
		  view.UntilMs = until.UnixMilli()
		  view.RemainingS = int64(until.Sub(now).Seconds())
		  if view.RemainingS < 0 {
			  view.RemainingS = 0
		   }
	   }
	   return view, nil
   }

// AnnotateSellBlocked 在摘要上叠加卖出是否被冷却拦住（只读）。

  func (f *Funds) AnnotateSellBlocked(ctx context.Context, userID string, sum OccupancySummary) (OccupancySummary, CoolingView, error) {
	  cool, err := f.CoolingState(ctx, userID)
	  if err != nil {
		  return sum, CoolingView{}, err
	   }
	  return sum, cool, nil
   }
