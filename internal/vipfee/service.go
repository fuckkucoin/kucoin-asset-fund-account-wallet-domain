package vipfee

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Service struct {
	mu sync.Mutex
	rows map[string]Schedule
	pins map[string]Pin
	fills []Fill
	seq    int
	window time.Duration
	promo map[Tier]string
	clock func() time.Time
	fees   *feeeng.Book
	sql    *persist.SQLDB
	genReg *generationRegistry
	storeBacked bool
}

   func New() *Service {
	   s := &Service{
		   rows:   map[string]Schedule{},
		  pins:   map[string]Pin{},
		   window: 30 * 24 * time.Hour,
		  promo: defaultPromo(),
		  clock: func() time.Time { return time.Now().UTC() },
		   fees: feeeng.DefaultBook(),
		  genReg: newGenerationRegistry(),
	  }
	  s.seedLocked()
	   return s
   }

  func defaultPromo() map[Tier]string {
	   return map[Tier]string{
		  TierStandard: "0",
		  TierVIP:      "100000",
		  TierVIPPlus: "1000000",
	  }
  }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	   return s.clock()
   }

   func (s *Service) BindFeeBook(b *feeeng.Book) *Service {
	  if s != nil && b != nil {
		   s.fees = b
	   }
	   return s
   }

   func (s *Service) SetClock(fn func() time.Time) {
	   if s != nil && fn != nil {
		   s.clock = fn
	  }
  }

  func (s *Service) SetWindow(d time.Duration) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   if d <= 0 {
		  return ErrSchedule
	  }
	   s.mu.Lock()
	   s.window = d
	   s.mu.Unlock()
	   return nil
   }

// ClearPromo 去掉进程内默认的成交量分档门槛。钉档和费率表行仍然生效。

  func (s *Service) ClearPromo() {
	  if s == nil {
		  return
	  }
	  s.mu.Lock()
	  s.promo = map[Tier]string{}
	  s.mu.Unlock()
  }

  func (s *Service) SetPromo(tier Tier, notional string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  t, err := ParseTier(string(tier))
	   if err != nil {
		   return err
	  }
	  if t == TierKYC0 {
		   return ErrTier
	  }
	   if _, err := parseBook(notional); err != nil {
		   return ErrAmount
	   }
	  s.mu.Lock()
	   s.promo[t] = notional
	   s.mu.Unlock()
	   return nil
   }
