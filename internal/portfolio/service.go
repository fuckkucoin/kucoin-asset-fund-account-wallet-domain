// @FileName: service.go
// @Summary: portfolio 包内 service 实现。
// @Author: Knox
// @Created: 2021.09.30

package portfolio

import (
	"context"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Service struct {
	led  *BookLedger
	acct *account.Service
}

   func New(acct *account.Service) *Service {
	   return &Service{led: NewBookLedger(), acct: acct}
  }

   func NewMem() *Service { return New(nil) }

  func (s *Service) Ledger() *BookLedger {
	   if s == nil {
		   return NewBookLedger()
	  }
	   if s.led == nil {
		   s.led = NewBookLedger()
	   }
	  return s.led
   }

   func (s *Service) Seed(userID, book, asset, avail, frozen string) (Position, error) {
	  return s.Ledger().Seed(userID,
		   book, asset, avail, frozen)
   }

   func (s *Service) Hop(userID string, from, to Book, asset, amount string) (HopResult, error) {
	   return s.Ledger().Hop(userID, from, to, asset, amount)
  }

   func (s *Service) HopIdem(userID string, from, to Book, asset, amount, key string) (HopResult, error) {
	   return s.Ledger().HopIdem(userID, from, to, asset, amount, key)
  }

  func (s *Service) Snapshot(userID string) (Snap, error) {
	   return s.Ledger().Snapshot(userID)
  }

  func (s *Service) Plan(userID, asset string, targets []Target) ([]PlanStep, error) {
	   return PlanRebalance(s.Ledger(), userID, asset, targets)
   }

   func (s *Service) Apply(userID string, steps []PlanStep) ([]HopResult, error) {
	  return s.Ledger().ApplyPlan(userID, steps)
   }

// LoadFromLedger 把账本科目灌进子账。只读，失败即关，不得用空快照当零余额。

   func (s *Service) LoadFromLedger(ctx context.Context, userID string) (Snap, error) {
	   if s == nil || s.acct == nil {
		  return Snap{}, ErrUnavailable
	  }
	   got, err := s.acct.List(ctx, userID)
	   if err != nil {
		   return Snap{}, err
	  }
	   for _, b := range got.Books {
		   if b.Asset == "" {
			  continue
		   }
		  if _, err := s.led.Seed(userID, string(b.Book), b.Asset, b.Available, b.Frozen); err != nil {
			   return Snap{}, err
		   }
	  }
	  return s.led.Snapshot(userID)
   }

   func (s *Service) MoveOnLedger(ctx context.Context, userID string, from, to Book, asset, amount, idem string) (*ledger.Transfer, error) {
	   if s == nil || s.acct == nil {
		   return nil, ErrUnavailable
	  }
	   amt, err := money.Parse(amount)
	   if err != nil {
		  return nil, err
	  }
	  if _, err := HopDir(from, to); err != nil {
		   return nil, err
	  }
	  return s.acct.Move(ctx, account.MoveReq{
		  UserID: userID, From: from, To: to, Asset: asset,
		  Amount: amt.Dec(), IdempotencyKey: idem,
	  })
  }
