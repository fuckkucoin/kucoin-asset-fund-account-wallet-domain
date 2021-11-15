// @FileName: service.go
// @Summary: 领域服务：业务用例编排与校验。
// @Author: Bronson
// @Created: 2021.11.14

package snap

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Service struct {
	store *Store
	led   *ledger.Service
	books ledger.Store
	clock func() time.Time
}

  func New(led *ledger.Service, books ledger.Store) *Service {
	  return &Service{
		   store: NewMemStore(), led: led, books: books,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func NewWithStore(led *ledger.Service, books ledger.Store, st *Store) *Service {
	  s := New(led, books)
	   if st != nil {
		  s.store = st
	  }
	   return s
  }

   func (s *Service) Store() *Store {
	   if s == nil {
		   return nil
	   }
	  return s.store
  }

  func (s *Service) WithClock(c func() time.Time) *Service {
	   if s != nil && c != nil {
		  s.clock = c
	   }
	  return s
   }

   // Capture 读现账并落快照。入参 userID；出参带围栏的 Snapshot。
   // 失败不写半截行。system 用户拒，避免把热冷库存混进用户余额。
  func (s *Service) Capture(ctx context.Context, userID string) (Snapshot, error) {
	  if s == nil {
		   return Snapshot{}, ErrUnavailable
	  }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return Snapshot{}, ErrUser
	  }
	  var snap Snapshot
	  var err error
	  if s.led != nil {
		   views, lerr := s.led.Accounts(ctx, userID)
		  if lerr != nil {
			  return Snapshot{}, lerr
		  }
		  snap, err = CaptureViews(userID, views, s.clock())
	  } else if s.books != nil {
		   snap, err = CaptureFrom(ctx, s.books, userID, s.clock())
	  } else {
		  return Snapshot{}, ErrUnavailable
	   }
	  if err != nil {
		   return Snapshot{}, err
	  }
	  if s.store != nil {
		   return s.store.Put(ctx, snap)
	  }
	  return snap, nil
  }

  func (s *Service) Get(ctx context.Context, id string) (Snapshot, error) {
	  if s == nil || s.store == nil {
		   return Snapshot{}, ErrUnavailable
	   }
	   return s.store.Get(ctx, id)
  }

  func (s *Service) Latest(ctx context.Context, userID string) (Snapshot, error) {
	   if s == nil || s.store == nil {
		   return Snapshot{}, ErrUnavailable
	   }
	  return s.store.Latest(ctx,
		   userID)
  }

  func (s *Service) List(ctx context.Context, userID string, limit int) ([]Snapshot, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	   return s.store.List(ctx, userID, limit)
  }

  func (s *Service) Diff(ctx context.Context, fromID, toID string) (Diff, error) {
	  a, err := s.Get(ctx, fromID)
	   if err != nil {
		  return Diff{}, err
	   }
	   b, err := s.Get(ctx, toID)
	   if err != nil {
		  return Diff{}, err
	   }
	   if !b.TakenAt.Before(a.TakenAt) {
		  return Compare(a, b)
	  }
	   return Diff{}, ErrClock
  }

// CheckLive 把已落快照和现账一致。Checksum 先过；科目金额不一致返回 ErrMismatch。
  func (s *Service) CheckLive(ctx context.Context, id string) error {
	  snap, err := s.Get(ctx, id)
	   if err != nil {
		  return err
	   }
	  if err := snap.VerifyChecksum(); err != nil {
		   return err
	   }
	   live, err := s.Capture(ctx, snap.UserID)
	  if err != nil {
		  return err
	  }
	  diff, err := Compare(snap, live)
	   if err != nil {
		  return err
	   }
	   if !diff.Empty() {
		  return fmt.Errorf("%w: %d changed %d added %d removed", ErrMismatch, len(diff.Changed), len(diff.Added), len(diff.Removed))
	  }
	   return nil
  }

// Totals 按资产汇总可用+冻结，给对账页脚。
   func (s Snapshot) Totals() (map[string]string, error) {
	   by := map[string]money.Amount{}
	   for _, p := range s.Positions {
		  t, err := p.TotalAmt()
		   if err != nil {
			  return nil, err
		   }
		  cur := by[p.Asset]
		  sum, err := cur.Add(t)
		   if err != nil {
			  return nil, err
		   }
		   by[p.Asset] = sum
	  }
	   out := make(map[string]string, len(by))
	  for asset, amt := range by {
		  out[asset] = amt.String()
	   }
	   return out, nil
   }
