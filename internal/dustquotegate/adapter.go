package dustquotegate

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/dustconv"
)

// StaticSource 单测/独立进程默认牌价。

type StaticSource struct {
	mu sync.Mutex
	rates map[string]string
	gens map[string]int64
	caps map[string]int64
	floors map[string]string
	mins  map[string]string
	books map[string]string
}

// NewStaticSource 预置 BTC/ETH。

   func NewStaticSource() *StaticSource {
	   return &StaticSource{
		   rates:  map[string]string{"BTC": "60000", "ETH": "2000"},
		   gens:   map[string]int64{"BTC": 1, "ETH": 1},
		   caps:   map[string]int64{"BTC": 50, "ETH": 50},
		  floors: map[string]string{"BTC": "0.0001", "ETH": "0.001"},
		   mins:   map[string]string{"BTC": "0.00000001", "ETH": "0.000001"},
		   books:  map[string]string{},
	   }
  }

  func bookK(user, asset string) string { return user + "|" + strings.ToUpper(asset) }

  func (s *StaticSource) LiveQuote(user, asset string) (LiveQuote, error) {
	   if s == nil {
		   return LiveQuote{},
			  ErrQuoteSnapshot
	   }
	  as := strings.ToUpper(strings.TrimSpace(asset))
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   rt, ok := s.rates[as]
	   if !ok {
		  return LiveQuote{},
			  ErrQuoteSnapshot
	  }
	   return LiveQuote{
		   Asset: as, Rate: rt, PolicyGen: s.gens[as], CapBPS: s.caps[as],
		  Floor: s.floors[as],
		  MinConv: s.mins[as], BookAvail: orZ(s.books[bookK(user, as)]),
		  At: time.Now().UTC(),
	   }, nil
  }

   func (s *StaticSource) PolicyGenOf(asset string) (int64, error) {
	  if s == nil {
		  return 0, ErrQuoteSnapshot
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.gens[strings.ToUpper(strings.TrimSpace(asset))], nil
  }

  func (s *StaticSource) RateOf(asset string) (string, error) {
	   if s == nil {
		   return "", ErrQuoteSnapshot
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  rt, ok := s.rates[strings.ToUpper(strings.TrimSpace(asset))]
	   if !ok {
		   return "", ErrQuoteSnapshot
	   }
	  return rt, nil
  }

   func (s *StaticSource) CapBPSOf(asset string) (int64, error) {
	   if s == nil {
		   return 0, ErrQuoteSnapshot
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.caps[strings.ToUpper(strings.TrimSpace(asset))],
		  nil
   }

// PutRate 改牌价（不自动吊销，由 Engine.OnFlip 负责）。
   func (s *StaticSource) PutRate(asset, rate string) {
	   if s == nil {
		   return
	  }
	   s.mu.Lock()
	   s.rates[strings.ToUpper(strings.TrimSpace(asset))] = rate
	  s.mu.Unlock()
   }

// PutPolicyGen 抬升世代。
   func (s *StaticSource) PutPolicyGen(asset string, gen int64) {
	   if s == nil {
		   return
	   }
	  s.mu.Lock()
	   s.gens[strings.ToUpper(strings.TrimSpace(asset))] = gen
	   s.mu.Unlock()
   }

// PutBook 钉扎账面。

  func (s *StaticSource) PutBook(user, asset, avail string) {
	   if s == nil {
		  return
	   }
	   s.mu.Lock()
	  s.books[bookK(user, asset)] = avail
	   s.mu.Unlock()
  }

  func orZ(s string) string {
	  if strings.TrimSpace(s) == "" {
		  return "0"
	  }
	   return s
  }

// DustconvSource 挂钩业务 dustconv.Service。
type DustconvSource struct {
	dust *dustconv.Service
}

// NewDustconvSource 包装。
   func NewDustconvSource(d *dustconv.Service) *DustconvSource {
	   return &DustconvSource{dust: d}
  }

  func (s *DustconvSource) LiveQuote(user, asset string) (LiveQuote, error) {
	   if s == nil || s.dust == nil {
		   return LiveQuote{}, ErrQuoteSnapshot
	   }
	   snap, err := s.dust.LiveLeaseQuote(user, asset)
	   if err != nil {
		  return LiveQuote{},
			   ErrQuoteSnapshot
	  }
	  return LiveQuote{
		  Asset: snap.Asset, Rate: snap.Rate, PolicyGen: snap.PolicyGen, CapBPS: snap.CapBPS,
		  Floor: snap.Floor, MinConv: snap.MinConvert, BookAvail: snap.BookAvail, At: snap.At,
	   }, nil
  }

  func (s *DustconvSource) PolicyGenOf(asset string) (int64, error) {
	  if s == nil || s.dust == nil {
		  return 0, ErrQuoteSnapshot
	  }
	  return s.dust.LeasePolicyGen(asset)
  }

   func (s *DustconvSource) RateOf(asset string) (string, error) {
	  if s == nil || s.dust == nil {
		   return "", ErrQuoteSnapshot
	   }
	  return s.dust.RateOf(asset)
  }

  func (s *DustconvSource) CapBPSOf(asset string) (int64, error) {
	   if s == nil || s.dust == nil {
		  return 0, ErrQuoteSnapshot
	  }
	  return s.dust.CapBPSOf(asset)
  }
