package usage

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Kind string

const (
	KindWithdraw Kind = "withdraw"
	KindDeposit  Kind = "deposit"
	KindTransfer Kind = "transfer"
)

type hit struct {
	Kind   Kind
	UserID string
	Asset  string
	Dest   string
	Amount money.Amount
	At time.Time
}

type Store struct {
	mu   sync.Mutex
	hits []hit
	clock func() time.Time
}

   func New() *Store {
	  return &Store{clock: func() time.Time { return time.Now().UTC() }}
  }

  func (s *Store) WithClock(c func() time.Time) *Store {
	  if s != nil && c != nil {
		   s.clock = c
	   }
	   return s
   }

   // Add 记一笔已接受的占用。amount 必须已过 money.Parse。dest 作出金目标去重用。

  func (s *Store) Add(kind Kind, userID, asset, dest, amount string) error {
	   if s == nil {
		  return nil
	  }
	   amt, err := money.Parse(amount)
	   if err != nil {
		   return err
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  s.hits = append(s.hits, hit{
		   Kind: kind, UserID: userID, Asset: asset, Dest: dest, Amount: amt, At: s.clock(),
	  })
	   return nil
   }

  func (s *Store) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
   }

// TodaySum 当日 00:00 UTC 起同 kind+user+asset 的合计。给 daily cap。
   func (s *Store) TodaySum(kind Kind, userID, asset string) money.Amount {
	   if s == nil {
		   return money.Zero()
	  }
	  now := s.now()
	   start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  sum := money.Zero()
	   for _, h := range s.hits {
		   if h.Kind != kind || h.UserID != userID || h.Asset != asset {
			   continue
		   }
		  if h.At.Before(start) {
			   continue
		   }
		  var err error
		  sum, err = sum.Add(h.Amount)
		   if err != nil {
			  return sum
		   }
	  }
	   return sum
   }

// Velocity 窗口内笔数。窗口外的命中不算。
  func (s *Store) Velocity(kind Kind, userID, asset string, win time.Duration) int {
	  if s == nil {
		  return 0
	  }
	   if win <= 0 {
		   win = time.Minute
	   }
	  cut := s.now().Add(-win)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  n := 0
	   for _, h := range s.hits {
		   if h.Kind != kind || h.UserID != userID || h.Asset != asset {
			   continue
		   }
		  if h.At.Before(cut) {
			  continue
		   }
		   n++
	   }
	  return n
  }

// DestCount 窗口内同一出金目标出现次数。0 表示首提该地址。

  func (s *Store) DestCount(userID, dest string, win time.Duration) int {
	  if s == nil || dest == "" {
		  return 0
	   }
	   if win <= 0 {
		  win = 30 * 24 * time.Hour
	   }
	   cut := s.now().Add(-win)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   n := 0
	   for _, h := range s.hits {
		   if h.Kind != KindWithdraw || h.UserID != userID || h.Dest != dest {
			   continue
		  }
		   if h.At.Before(cut) {
			  continue
		  }
		  n++
	   }
	   return n
   }

   func (s *Store) Len() int {
	  if s == nil {
		   return 0
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  return len(s.hits)
   }
