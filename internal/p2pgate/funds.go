package p2pgate

import (
	"context"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/shopspring/decimal"
)

var (
	ErrOrder = errors.New("p2p funding order id required")
	ErrHold = errors.New("p2p funding hold unavailable")
	ErrCooling = errors.New("p2p sell blocked: withdraw cooling active")
)

type Funds struct {
	led *ledger.Service
	hold *holdcat.Book
}

   func NewFunds(led *ledger.Service, hold *holdcat.Book) *Funds {
	   return &Funds{led: led, hold: hold}
  }

  func (f *Funds) Buy(ctx context.Context, userID, asset, amount, orderID string) (*ledger.Transfer, error) {
	   uid, as, amt, err := parseFund(userID, asset, amount)
	   if err != nil {
		   return nil, err
	  }
	   orderID = strings.TrimSpace(orderID)
	  if orderID == "" {
		  return nil, ErrOrder
	   }
	  if f == nil || f.led == nil {
		   return nil, ErrUnavailable
	  }
	   return f.led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.P2PCredit, Asset: as, Amount: amt,
		   IdempotencyKey: "p2p-buy:" + orderID,
	  })
   }

   func (f *Funds) SellHold(ctx context.Context, userID, asset, amount, orderID string) (holdcat.Row, error) {
	  uid, as, amt, err := parseFund(userID, asset, amount)
	  if err != nil {
		  return holdcat.Row{}, err
	   }
	  orderID = strings.TrimSpace(orderID)
	  if orderID == "" {
		  return holdcat.Row{}, ErrOrder
	  }
	  if f == nil || f.led == nil || f.hold == nil {
		  return holdcat.Row{}, ErrHold
	   }
	   // 改交易密码等安全变更后 24h 禁 P2P 卖出（复用出金冷却）。
	   if err := f.requireCoolingClear(ctx, uid); err != nil {
		  return holdcat.Row{}, err
	   }
	  if _, err := f.led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.P2PDebit, Asset: as, Amount: amt,
		   IdempotencyKey: "p2p-sell:" + orderID,
	  }); err != nil {
		   return holdcat.Row{}, err
	   }
	  return f.hold.Place(ctx, uid, as, "funding", amt.String(), holdcat.ReasonP2PSell, orderID)
  }

   func (f *Funds) MarginHold(ctx context.Context, userID, asset, amount, orderID string) (holdcat.Row, error) {
	  uid, as, amt, err := parseFund(userID, asset, amount)
	  if err != nil {
		   return holdcat.Row{}, err
	   }
	   orderID = strings.TrimSpace(orderID)
	   if orderID == "" {
		  return holdcat.Row{}, ErrOrder
	  }
	   if f == nil || f.led == nil || f.hold == nil {
		  return holdcat.Row{}, ErrHold
	   }
	   if _, err := f.led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.P2PDebit, Asset: as, Amount: amt,
		   IdempotencyKey: "p2p-margin:" + orderID,
	  }); err != nil {
		   return holdcat.Row{}, err
	  }
	  return f.hold.Place(ctx, uid, as, "funding", amt.String(), holdcat.ReasonP2PMargin, orderID)
  }

  func (f *Funds) Release(ctx context.Context, userID, holdID string) (holdcat.Row, error) {
	  if f == nil || f.hold == nil {
		   return holdcat.Row{}, ErrHold
	  }
	  row, err := f.hold.Get(ctx, userID, holdID)
	  if err != nil {
		  return holdcat.Row{}, err
	   }
	   if !row.Open {
		  return row, nil
	  }
	   if f.led != nil && (row.Reason == holdcat.ReasonP2PSell || row.Reason == holdcat.ReasonP2PMargin) {
		   amt, err := decimal.NewFromString(row.Amount)
		   if err != nil || !amt.IsPositive() {
			  return holdcat.Row{}, ErrAmount
		   }
		   if _, err := f.led.Create(ctx, ledger.CreateReq{
			  UserID: row.UserID, Direction: ledger.P2PCredit, Asset: row.Asset, Amount: amt,
			   IdempotencyKey: "p2p-rel:" + row.ID,
		   }); err != nil {
			   return holdcat.Row{}, err
		   }
	  }
	  return f.hold.Release(ctx, userID, holdID)
  }

// requireCoolingClear 出金冷却期内拒绝 P2P 卖出占用。
// 与 redenvelope 同口径读 WithdrawLockUntil。
// 冷却查询失败不得放行。
  func (f *Funds) requireCoolingClear(ctx context.Context, userID string) error {
	  if f == nil || f.led == nil {
		  return ErrHold
	  }
	   until,
		   _, err := f.led.WithdrawLockUntil(ctx, userID)
	  if err != nil {
		   return err
	   }
	   if !until.IsZero() && until.After(time.Now().UTC()) {
		   return ErrCooling
	  }
	   return nil
   }

// ListOpen 仅返回 P2P 卖出/商家保证金占用行（资金页邻接）。

   func (f *Funds) ListOpen(ctx context.Context, userID string) ([]holdcat.Row, error) {
	   if f == nil || f.hold == nil {
		  return nil, ErrHold
	  }
	  rows, err := f.hold.List(ctx, userID, true)
	   if err != nil {
		  return nil, err
	  }
	   out := make([]holdcat.Row,
		  0, len(rows))
	   for _, r := range rows {
		  if r.Reason == holdcat.ReasonP2PSell || r.Reason == holdcat.ReasonP2PMargin {
			   out = append(out, r)
		   }
	   }
	   return out, nil
   }

  func parseFund(userID, asset, amount string) (string, string, decimal.Decimal, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return "", "", decimal.Zero, err
	  }
	   as, err := validate.Asset(asset)
	  if err != nil {
		  return "", "", decimal.Zero, err
	  }
	  amt, err := ledger.ParseAmount(amount)
	   if err != nil {
		  return "", "", decimal.Zero, ErrAmount
	  }
	  return uid, as, amt, nil
   }
