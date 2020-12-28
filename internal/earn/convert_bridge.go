package earn

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/cvfund"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// ConvertSubscribeReq 闪兑目标币后直申购理财。
// 市价闪兑入资金账再 Subscribe；两段幂等键分离可对账。
// 闪兑成功申购失败时资金留资金账，不得自动重兑。
type ConvertSubscribeReq struct {
	UserID       string `json:"user_id"`
	FromCurrency string `json:"from_currency"`
	FromAmount  string `json:"from_amount"`
	ProductID   string `json:"product_id"`
	AccountType string `json:"account_type"`
	IdempotencyKey string `json:"idempotency_key"`
	AutoSubscribe bool `json:"auto_subscribe"`
}

// ConvertSubscribeResult 返回理财仓位与闪兑单，便于上层对账。

type ConvertSubscribeResult struct {
	Position Position     `json:"position"`
	Convert  cvfund.Order `json:"convert"`
	ToAmount string       `json:"to_amount"`
}

// ConvertSubscribe 闪兑邻接申购。

   func (s *Service) ConvertSubscribe(ctx context.Context, cv *cvfund.Book, req ConvertSubscribeReq) (ConvertSubscribeResult, error) {
	  out := ConvertSubscribeResult{}
	   if s == nil {
		   return out, ErrUnavailable
	   }
	   if cv == nil {
		  return out, fmt.Errorf("convert book unavailable")
	  }
	  idem := strings.TrimSpace(req.IdempotencyKey)
	  if idem == "" {
		   return out, fmt.Errorf("idempotency_key required")
	   }
	  prod, err := s.Catalog().Get(req.ProductID)
	  if err != nil {
		  return out, err
	  }
	  to := strings.ToUpper(strings.TrimSpace(prod.Asset))
	   from := strings.ToUpper(strings.TrimSpace(req.FromCurrency))
	   if from == "" || to == "" || from == to {
		  return out, fmt.Errorf("currency pair invalid")
	   }
	  acct := strings.ToUpper(strings.TrimSpace(req.AccountType))
	   if acct == "" {
		  acct = "MAIN"
	  }
	  book := mapConvertBook(acct)
	   ord, err := cv.PlaceMarket(ctx, req.UserID, from, to, req.FromAmount, "", book)
	   if err != nil {
		  return out, err
	   }
	  out.Convert = ord
	  src,
		  err := decimal.NewFromString(strings.TrimSpace(ord.Amount))
	   if err != nil || !src.IsPositive() {
		  src, err = decimal.NewFromString(strings.TrimSpace(req.FromAmount))
		  if err != nil || !src.IsPositive() {
			  return out, fmt.Errorf("convert source amount invalid")
		   }
	   }
	   rate, err := decimal.NewFromString(strings.TrimSpace(ord.Rate))
	  if err != nil || !rate.IsPositive() {
		  return out, fmt.Errorf("convert rate invalid")
	  }
	   got := src.Mul(rate)
	  amt := got.String()
	   if _, err := money.Parse(amt); err != nil {
		  return out, fmt.Errorf("convert result amount invalid: %w", err)
	  }
	  out.ToAmount = amt
	  pos, err := s.Subscribe(ctx, SubscribeReq{
		  UserID:    req.UserID,
		  ProductID: req.ProductID, Asset: to, Amount: amt,
		  IdempotencyKey: idem + "-earn", AutoSubscribe: req.AutoSubscribe,
	  })
	   if err != nil {
		  return out, err
	  }
	  out.Position = pos
	   return out, nil
   }

   func mapConvertBook(accountType string) string {
	   switch strings.ToUpper(strings.TrimSpace(accountType)) {
	  case "TRADE", "TRADING":
		   return "trading"
	  case "BOTH":
		  return "both"
	   default:
		  return "funding"
	  }
   }
