package lending

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// LendingAdapter 将 lending.Service 适配为 lendoccupygate.LendingService 接口。
type LendingAdapter struct {
	svc *Service
}

// NewLendingAdapter 构造适配器。
  func NewLendingAdapter(svc *Service) *LendingAdapter {
	  return &LendingAdapter{svc: svc}
  }

// Place 放贷挂单（P2PDebit + holdcat）；返回 holdRef、idemKey 及错误。

  func (a *LendingAdapter) Place(ctx context.Context, userID, asset, amount, orderID string) (holdRef string, idemKey string, holdErr error) {
	   row, err := a.svc.Place(ctx, userID, asset, amount, orderID)
	   if err != nil {
		  return "", "", err
	  }
	  return row.Ref, "lend-ord:" + orderID, nil
   }

// Settle 成交释放（P2PCredit + ReleaseByRef）。
  func (a *LendingAdapter) Settle(ctx context.Context, userID, asset, amount, orderID string) error {
	   _, err := a.svc.Settle(ctx, userID, asset, amount, orderID)
	  return err
  }

// Cancel 取消退回（与 Settle 等价，幂等键独立）。

   func (a *LendingAdapter) Cancel(ctx context.Context, userID, asset, amount, orderID string) error {
	   _, err := a.svc.Cancel(ctx, userID, asset, amount, orderID)
	  return err
  }

// CompensateDebit 补偿冲销借记（P2PCredit，幂等键 "lend-residue-comp:" + orderID）。
   func (a *LendingAdapter) CompensateDebit(ctx context.Context, userID, asset, amount, orderID string) error {
	   uid, as, amt, err := parse(userID, asset, amount)
	   if err != nil {
		   return err
	  }
	  _, err = a.svc.led.Create(ctx, ledger.CreateReq{
		   UserID:    uid,
		   Direction: ledger.P2PCredit,
		   Asset:     as,
		   Amount: amt,
		  IdempotencyKey: "lend-residue-comp:" + orderID,
	  })
	  return err
  }

// Cancel 取消放贷挂单退回：P2PCredit 回 Funding，并按 order id ReleaseByRef。
// 与 Settle 使用独立幂等键（"lend-cancel:" + orderID），防止与成交混淆。
   func (s *Service) Cancel(ctx context.Context, userID, asset, amount, orderID string) (holdcat.Row, error) {
	  if s == nil || s.led == nil || s.holds == nil {
		  return holdcat.Row{}, ErrUnavailable
	   }
	   uid,
		   as, amt, err := parse(userID, asset, amount)
	   if err != nil {
		  return holdcat.Row{}, err
	  }
	  orderID = strings.TrimSpace(orderID)
	   if orderID == "" {
		   return holdcat.Row{}, ErrOrder
	   }
	  if _, err := s.led.Create(ctx, ledger.CreateReq{
		   UserID: uid,
		   Direction:      ledger.P2PCredit,
		   Asset:          as,
		  Amount:         amt,
		  IdempotencyKey: "lend-cancel:" + orderID,
	  }); err != nil {
		   return holdcat.Row{}, err
	  }
	   return s.holds.ReleaseByRef(ctx, uid, holdcat.ReasonLendingOrder, orderID)
  }
