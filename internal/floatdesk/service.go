// 领域服务：业务用例编排与校验。

// Author: Cason
// Date: 2021.05.23

package floatdesk

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
)

type Service struct {
	cat *Catalog
	tanks *Tanks
	halt   *Flags
	refill *RefillObserve
	mu     sync.Mutex
	samples []Sample
	clock   func() time.Time
}

   func New() *Service {
	  return &Service{
		  cat: DefaultCatalog(), tanks: DefaultTanks(), halt: NewFlags(),
		  refill: NewRefillObserve(),
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
   }

   func (s *Service) Catalog() *Catalog {
	   if s == nil {
		   return NewCatalog()
	  }
	   return s.cat
  }

   func (s *Service) Tanks() *Tanks {
	  if s == nil {
		  return NewTanks()
	   }
	   return s.tanks
   }

   func (s *Service) Halt() *Flags {
	  if s == nil {
		   return NewFlags()
	   }
	   return s.halt
  }

// RefillObserve 补仓进行中只读挂钩。不暴露代次锁写接口。
   func (s *Service) RefillObserve() *RefillObserve {
	  if s == nil {
		   return NewRefillObserve()
	   }
	   return s.refill
   }

// Refill leftover 只读补仓标记入口（不写代次锁内核）。

   func (s *Service) Refill() *RefillObserve {
	  return s.RefillObserve()
  }

// SetClock 时钟。

  func (s *Service) SetClock(fn func() time.Time) {
	  if s == nil || fn == nil {
		   return
	  }
	   s.clock = fn
  }

   func (s *Service) Record(sm Sample) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   got, _, err := parseSample(sm)
	  if err != nil {
		  return err
	  }
	   if err := s.halt.Check(got.Chain, got.Asset); err != nil {
		   return err
	  }
	  s.mu.Lock()
	  s.samples = append(s.samples, got)
	   s.mu.Unlock()
	   return nil
  }

  func (s *Service) Samples() []Sample {
	  if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Sample, len(s.samples))
	   copy(out, s.samples)
	  return out
  }

  func (s *Service) Outlook(chain, asset, hot string, window time.Duration) (Outlook, error) {
	  if s == nil {
		   return Outlook{}, ErrUnavailable
	   }
	   if err := s.halt.Check(chain, asset); err != nil {
		  return Outlook{}, err
	  }
	  return Forecast(s.Samples(), chain, asset, hot, window, s.clock())
   }

   func (s *Service) OutlookFromInv(inv []hotinv.Snapshot, window time.Duration) ([]Outlook, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   out := make([]Outlook, 0)
	   for _, snap := range inv {
		  for _, t := range s.cat.List() {
			   if t.Asset != snap.Asset {
				   continue
			   }
			  got,
				   err := s.Outlook(t.Chain, t.Asset, snap.HotAvail, window)
			   if err != nil {
				   return nil, err
			   }
			  out = append(out, got)
		  }
	  }
	   return out, nil
  }

   func (s *Service) GasRefill() ([]Refill, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   if err := s.halt.Check("", ""); err != nil && s.halt.Halted() {
		   return nil, err
	   }
	  rows := s.tanks.ScanRefill()
	   kept := make([]Refill, 0, len(rows))
	  for _, r := range rows {
		   if err := s.halt.Check(r.Chain, r.Native); err != nil {
			  continue
		   }
		  kept = append(kept, r)
	   }
	   return kept, nil
  }
