package dustconv

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// LeaseQuote Convert 预检钉扎的活牌价视图（与 dustleak QuoteSnapshot 分立，避免同包类型碰撞）。
type LeaseQuote struct {
	Asset string `json:"asset"`
	Rate      string `json:"rate"`
	PolicyGen int64  `json:"policy_gen"`
	CapBPS int64 `json:"cap_bps"`
	Floor      string `json:"floor_pinned"`
	MinConvert string `json:"min_convert_pinned"`
	BookAvail string `json:"book_available"`
	At time.Time `json:"at"`
}

// Copy 值拷贝。

   func (q LeaseQuote) Copy() LeaseQuote { return q }

// SameLeaseQuote 与活快照；不改公式，只判漂移。
   func (q LeaseQuote) SameLeaseQuote(live LeaseQuote) error {
	  if strings.TrimSpace(q.Rate) != strings.TrimSpace(live.Rate) {
		  return ErrRate
	  }
	   if q.PolicyGen != live.PolicyGen {
		   return ErrPolicy
	  }
	   if q.CapBPS != live.CapBPS {
		   return ErrPolicy
	  }
	  return nil
   }

// LiveLeaseQuote 读取用户资产活牌价。

  func (s *Service) LiveLeaseQuote(user, asset string) (LeaseQuote, error) {
	  if s == nil {
		   return LeaseQuote{}, ErrUnavailable
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return LeaseQuote{}, ErrAsset
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   pol := s.policy[as]
	  if pol == nil || !pol.Enabled {
		   return LeaseQuote{}, ErrPolicy
	   }
	   rt := s.rates[as]
	  if rt == nil {
		  return LeaseQuote{}, ErrRate
	   }
	   if _, err := money.Parse(rt.USDT); err != nil {
		   return LeaseQuote{}, ErrRate
	   }
	  book := s.books[bookKey(strings.TrimSpace(user), as)]
	  avail := "0"
	  if book != nil {
		  avail = orZ(book.Available)
	   }
	   gen := int64(1)
	  if s.policyGen != nil && s.policyGen[as] != 0 {
		   gen = s.policyGen[as]
	  }
	  cap := s.capBPS[as]
	  if cap == 0 {
		  cap = 50
	  }
	   return LeaseQuote{
		  Asset: as, Rate: rt.USDT, PolicyGen: gen, CapBPS: cap,
		   Floor:      pol.Floor,
		   MinConvert: pol.MinConvert, BookAvail: avail, At: s.now(),
	  }, nil
  }

// LeasePolicyGen 当前租约侧策略世代（与 PolicyGen 快照类型分立）。
   func (s *Service) LeasePolicyGen(asset string) (int64, error) {
	  if s == nil {
		  return 0, ErrUnavailable
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return 0, ErrAsset
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  g := s.policyGen[as]
	  if g == 0 {
		  return 1, nil
	   }
	  return g, nil
   }

// RateOf 活牌价字符串。
   func (s *Service) RateOf(asset string) (string, error) {
	   if s == nil {
		  return "", ErrUnavailable
	  }
	  as, err := validate.Asset(asset)
	   if err != nil {
		   return "", ErrAsset
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  rt := s.rates[as]
	  if rt == nil {
		  return "", ErrRate
	  }
	   return rt.USDT, nil
  }

// CapBPSOf 钉扎 cap（不改公式，只读）。
   func (s *Service) CapBPSOf(asset string) (int64, error) {
	  if s == nil {
		   return 0, ErrUnavailable
	   }
	  as, err := validate.Asset(asset)
	   if err != nil {
		  return 0, ErrAsset
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  c := s.capBPS[as]
	   if c == 0 {
		  return 50, nil
	   }
	  return c, nil
  }

// SetLeasePolicyGen 抬升租约侧策略世代（旧租约必须失效）。
  func (s *Service) SetLeasePolicyGen(asset string, gen int64) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return ErrAsset
	   }
	   if gen <= 0 {
		   return ErrPolicy
	   }
	   s.mu.Lock()
	   if s.policyGen == nil {
		   s.policyGen = map[string]int64{}
	   }
	   old := s.policyGen[as]
	  s.policyGen[as] = gen
	   hook := s.onPolicyFlip
	  s.mu.Unlock()
	   if hook != nil {
		   hook(as, old, gen)
	   }
	   return nil
   }

// PutCapBPS 仅钉扎 cap_bps，不改写 floor/cap 业务公式。
   func (s *Service) PutCapBPS(asset string, bps int64) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return ErrAsset
	   }
	   if bps <= 0 {
		   return ErrPolicy
	  }
	   s.mu.Lock()
	   s.capBPS[as] = bps
	  s.mu.Unlock()
	   return nil
   }
