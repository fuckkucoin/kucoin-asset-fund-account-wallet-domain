// @FileName: service.go
// @Summary: 放贷未成交订单的资金占用。
// 挂单借记 Funding→bridge 并写 holdcat lending_order；成交/取消贷记并 ReleaseByRef。
// 撮合属交易邻接，本包只保证资金页冻结原因可解释与账本守恒。
// @Author: Cason
// @Created: 2021.08.22

package lending

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/shopspring/decimal"
)

var (
	ErrUnavailable = errors.New("lending funding unavailable")
	ErrOrder  = errors.New("lending order id required")
	ErrAmount = errors.New("lending amount invalid")
)

// Service 放贷资金占用。须绑定 ledger 与 holdcat。

type Service struct {
	led   *ledger.Service
	holds *holdcat.Book
}

  func New(led *ledger.Service, holds *holdcat.Book) *Service {
	  return &Service{led: led,
		   holds: holds}
   }

// Place 放贷挂单：P2PDebit 入 bridge 后写 lending_order 标签。
   func (s *Service) Place(ctx context.Context, userID, asset, amount, orderID string) (holdcat.Row, error) {
	  if s == nil || s.led == nil || s.holds == nil {
		  return holdcat.Row{}, ErrUnavailable
	   }
	  uid, as, amt, err := parse(userID, asset, amount)
	   if err != nil {
		   return holdcat.Row{}, err
	  }
	   orderID = strings.TrimSpace(orderID)
	  if orderID == "" {
		  return holdcat.Row{}, ErrOrder
	   }
	   if _, err := s.led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.P2PDebit, Asset: as, Amount: amt,
		   IdempotencyKey: "lend-ord:" + orderID,
	  }); err != nil {
		  return holdcat.Row{}, err
	   }
	  return s.holds.PlaceLendingOrder(ctx, uid, as, "funding", amt.String(), orderID)
  }

// Settle 成交入借出账或取消退回：P2PCredit 回 Funding，并按 order id ReleaseByRef。

  func (s *Service) Settle(ctx context.Context, userID, asset, amount, orderID string) (holdcat.Row, error) {
	  if s == nil || s.led == nil || s.holds == nil {
		  return holdcat.Row{}, ErrUnavailable
	   }
	   uid, as, amt, err := parse(userID, asset, amount)
	   if err != nil {
		   return holdcat.Row{}, err
	   }
	  orderID = strings.TrimSpace(orderID)
	  if orderID == "" {
		   return holdcat.Row{}, ErrOrder
	  }
	   if _, err := s.led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.P2PCredit, Asset: as, Amount: amt,
		  IdempotencyKey: "lend-settle:" + orderID,
	  }); err != nil {
		   return holdcat.Row{}, err
	   }
	   return s.holds.ReleaseByRef(ctx, uid, holdcat.ReasonLendingOrder, orderID)
   }

   func parse(userID, asset, amount string) (string, string, decimal.Decimal, error) {
	   uid,
		  err := validate.UserID(userID)
	  if err != nil {
		   return "", "", decimal.Zero, err
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return "", "", decimal.Zero, err
	   }
	   amt,
		   err := ledger.ParseAmount(amount)
	  if err != nil {
		  return "", "", decimal.Zero, ErrAmount
	  }
	   return uid, as, amt, nil
  }
