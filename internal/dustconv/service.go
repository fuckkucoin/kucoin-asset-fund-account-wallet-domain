package dustconv

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// FlipHook 牌价/策略世代翻转回调（尘兑租约闸吊销泵）。
type FlipHook func(asset string, oldV, newV string)

// PolicyFlipHook 策略世代抬升回调。

type PolicyFlipHook func(asset string, oldGen, newGen int64)

type Service struct {
	mu     sync.Mutex
	policy map[string]*Policy
	rates map[string]*Rate
	books  map[string]*Book
	rows   map[string]*Convert
	byIdem map[string]string
	clock func() time.Time
	genCounters map[string]*genCounter // PolicyGen per-asset counters; pointer held by mu, increment via atomic
	policyGen   map[string]int64
	capBPS       map[string]int64
	onRateFlip   FlipHook
	onPolicyFlip PolicyFlipHook
	leaseGate LeaseGate
}

  func New() *Service {
	  s := &Service{
		   policy: map[string]*Policy{},
		   rates:  map[string]*Rate{},
		  books:  map[string]*Book{},
		  rows:   map[string]*Convert{},
		  byIdem: map[string]string{},
		   genCounters: map[string]*genCounter{},
		  policyGen:   map[string]int64{"BTC": 1, "ETH": 1},
		  capBPS:      map[string]int64{"BTC": 50, "ETH": 50},
		  clock:       func() time.Time { return time.Now().UTC() },
	  }
	  _ = s.PutPolicy(Policy{Asset: "BTC", Floor: "0.0001", MinConvert: "0.00000001", Enabled: true})
	  _ = s.PutPolicy(Policy{Asset: "ETH", Floor: "0.001", MinConvert: "0.000001", Enabled: true})
	  _ = s.PutRate("BTC", "60000")
	   _ = s.PutRate("ETH", "2000")
	   return s
  }

// BindLeaseGate 挂尘兑牌价租约闸（Convert 必须带 dust_quote_lease_id）。
   func (s *Service) BindLeaseGate(g LeaseGate) {
	  if s == nil {
		  return
	  }
	   s.mu.Lock()
	  s.leaseGate = g
	  s.mu.Unlock()
  }

// OnRateFlip 注册牌价翻转钩子。

  func (s *Service) OnRateFlip(h FlipHook) {
	  if s == nil {
		   return
	   }
	   s.mu.Lock()
	  s.onRateFlip = h
	  s.mu.Unlock()
  }

// OnPolicyFlip 注册策略世代钩子。
  func (s *Service) OnPolicyFlip(h PolicyFlipHook) {
	   if s == nil {
		   return
	   }
	   s.mu.Lock()
	  s.onPolicyFlip = h
	   s.mu.Unlock()
  }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock()
   }

   func bookKey(user, asset string) string { return user + "|" + asset }

  func (s *Service) PutPolicy(p Policy) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  as, err := validate.Asset(p.Asset)
	   if err != nil {
		  return ErrAsset
	  }
	  if _, err := money.Parse(p.Floor); err != nil {
		   return ErrFloor
	   }
	  if _, err := money.Parse(p.MinConvert); err != nil {
		  return ErrFloor
	   }
	   p.Asset = as
	  s.mu.Lock()
	  cp := p
	  s.policy[as] = &cp
	   s.mu.Unlock()
	   return nil
   }

   func (s *Service) PutRate(asset, usdt string) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	   as, err := validate.Asset(asset)
	  if err != nil {
		   return ErrAsset
	  }
	  if as == "USDT" {
		  return ErrRate
	  }
	   if _, err := money.Parse(usdt); err != nil {
		   return ErrRate
	   }
	   s.mu.Lock()
	  old := ""
	  if cur := s.rates[as]; cur != nil {
		   old = cur.USDT
	   }
	  s.rates[as] = &Rate{Asset: as, USDT: usdt}
	   hook := s.onRateFlip
	  s.mu.Unlock()
	  if hook != nil && old != "" && old != usdt {
		  hook(as, old, usdt)
	   }
	   return nil
  }

  func (s *Service) Credit(user, asset, amount string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	   uid,
		   err := validate.UserID(user)
	   if err != nil {
		   return ErrUser
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return ErrAsset
	  }
	  amt, err := money.Parse(amount)
	   if err != nil {
		   return ErrAmount
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.addLocked(uid, as, amt)
  }

  func (s *Service) BookOf(user, asset string) (Book, error) {
	   if s == nil {
		   return Book{}, ErrUnavailable
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return Book{}, ErrAsset
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cur := s.books[bookKey(strings.TrimSpace(user), as)]
	   if cur == nil {
		  return Book{UserID: strings.TrimSpace(user), Asset: as, Available: "0"}, nil
	   }
	  return cur.Copy(), nil
  }

   func (s *Service) addLocked(user, asset string, delta money.Amount) error {
	  key := bookKey(user, asset)
	  cur := s.books[key]
	   if cur == nil {
		   cur = &Book{UserID: user, Asset: asset, Available: "0"}
		  s.books[key] = cur
	  }
	  got, err := money.ParseBook(orZ(cur.Available))
	   if err != nil {
		  return ErrAmount
	  }
	   next, err := got.Add(delta)
	  if err != nil {
		  return err
	  }
	   cur.Available = next.String()
	   return nil
  }

  func (s *Service) subLocked(user, asset string, delta money.Amount) error {
	   key := bookKey(user, asset)
	  cur := s.books[key]
	  if cur == nil {
		  return ErrAmount
	   }
	   got, err := money.ParseBook(orZ(cur.Available))
	  if err != nil {
		   return ErrAmount
	   }
	   next, err := got.Sub(delta)
	   if err != nil {
		  return err
	  }
	   cur.Available = next.String()
	  return nil
   }

  func orZ(s string) string {
	   if strings.TrimSpace(s) == "" {
		   return "0"
	   }
	  return s
   }
