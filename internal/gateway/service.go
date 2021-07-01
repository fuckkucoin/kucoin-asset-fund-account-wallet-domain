// 统一余额和账单。/v1 走进程内 Bearer；未配 JWKS 时不验厂商 JWT。

// Author: TianpengLan
// Date: 2021.06.30

package gateway

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Service struct {
	led *ledger.Service
	w *wallet.Service
}

  func New(led *ledger.Service, w *wallet.Service) *Service {
	  return &Service{led: led, w: w}
  }

  func (s *Service) Unified(ctx context.Context, userID string) ([]wallet.UnifiedBalance, error) {
	  if s == nil || s.w == nil {
		   return nil, fmt.Errorf("asset query wallet unavailable")
	   }
	   rows, err := s.w.Balances(ctx, userID, "")
	   if err != nil {
		   return nil, err
	   }
	   return wallet.UnifyBalances(rows), nil
   }

   func (s *Service) Bills(ctx context.Context, userID string, limit int) ([]ledger.Bill, error) {
	  if s == nil || s.led == nil {
		  return nil, fmt.Errorf("asset query ledger unavailable")
	  }
	  return s.led.ListBills(ctx,
		  userID, limit)
  }

  func (s *Service) Accounts(ctx context.Context, userID string) ([]ledger.BookView, error) {
	   if s == nil || s.led == nil {
		  return nil, fmt.Errorf("asset query ledger unavailable")
	  }
	   return s.led.Accounts(ctx, userID)
   }

type BillQuery struct {
	UserID string
	Asset  string
	Book    string
	AppType string
	Limit   int
}

// Statement 用已落库流水滚 running。limit 截最新，running 按全量前缀算。

   func (s *Service) Statement(ctx context.Context, q BillQuery) (stmt.Report, error) {
	  if s == nil || s.led == nil {
		  return stmt.Report{}, fmt.Errorf("asset query ledger unavailable")
	   }
	  fetch := q.Limit
	  if fetch < 200 {
		   fetch = 200
	  }
	   bills, err := s.led.ListBills(ctx, q.UserID, fetch)
	  if err != nil {
		   return stmt.Report{}, err
	  }
	  return stmt.Build(q.UserID, bills, stmt.Filter{
		   Asset: q.Asset, Book: q.Book, AppType: q.AppType, Limit: q.Limit,
	   })
  }
