package p2pnet

import (
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Screen struct {
	mu sync.Mutex
	blocked   map[string]struct{}
	maxSingle string
}

   func NewScreen() *Screen {
	  return &Screen{blocked: map[string]struct{}{}, maxSingle: "2000"}
   }

   func (s *Screen) Block(userID string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return ErrUser
	   }
	   s.mu.Lock()
	  s.blocked[uid] = struct{}{}
	  s.mu.Unlock()
	   return nil
  }

  func (s *Screen) Unblock(userID string) {
	  if s == nil {
		  return
	  }
	  s.mu.Lock()
	   delete(s.blocked, strings.TrimSpace(userID))
	  s.mu.Unlock()
  }

  func (s *Screen) SetMax(amount string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	  if _, err := money.Parse(amount); err != nil {
		  return ErrAmount
	  }
	  s.mu.Lock()
	   s.maxSingle = amount
	   s.mu.Unlock()
	   return nil
  }

// Check 自己转自己、对手拉黑、超单笔上限都拒。金额必须过 money.Parse。
   func (s *Screen) Check(from, to, asset, amount string) ([]string, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  from, err := validate.UserID(from)
	   if err != nil {
		   return nil, ErrUser
	   }
	   to, err = validate.UserID(to)
	   if err != nil {
		   return nil, ErrDest
	   }
	   if from == to {
		  return nil, ErrSelf
	  }
	  if _, err := validate.Asset(asset); err != nil {
		   return nil, ErrAsset
	  }
	   amt, err := money.Parse(amount)
	  if err != nil {
		  return nil, ErrAmount
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.blocked[from]; ok {
		   return nil, ErrBlocked
	   }
	  if _, ok := s.blocked[to]; ok {
		  return nil, ErrBlocked
	  }
	   max, err := money.Parse(s.maxSingle)
	   if err != nil {
		   return nil, ErrRisk
	  }
	  if amt.Cmp(max) > 0 {
		  return nil, ErrRisk
	   }
	   flags := make([]string, 0, 1)
	   half := money.FromDecimal(max.Dec().Div(decimal.NewFromInt(2)))
	   if amt.Cmp(half) > 0 {
		  flags = append(flags, "large_send")
	  }
	   return flags, nil
  }

   func (s *Screen) Blocked() []string {
	  if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]string, 0, len(s.blocked))
	  for k := range s.blocked {
		   out = append(out, k)
	   }
	   return out
  }
