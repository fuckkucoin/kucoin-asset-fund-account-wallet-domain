// 功能：领域服务：业务用例编排与校验。
// 作者：Cason
// 日期：2021.09.04

package obsdash

import (
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/slo"
)

type Service struct {
	ctr *Counters
	burn *Burns
	led  *ledger.Service
	store ledger.Store
	user string
}

   func New() *Service {
	   return &Service{ctr: NewCounters(),
		  burn: NewBurns()}
   }

   func (s *Service) Bind(led *ledger.Service, store ledger.Store, user string) *Service {
	   if s != nil {
		   s.led = led
		   s.store = store
		   s.user = user
	  }
	  return s
   }

   func (s *Service) Counters() *Counters {
	   if s == nil {
		   return NewCounters()
	  }
	  return s.ctr
   }

   func (s *Service) Burns() *Burns {
	  if s == nil {
		  return NewBurns()
	  }
	   return s.burn
  }

  func (s *Service) Note(name, label string) {
	  if s == nil {
		  return
	   }
	  _ = s.ctr.Inc(name, label, 1)
  }

   func Documented() map[string]any {
	  d := slo.Documented()
	  d["fund_safety"] = map[string]any{
		   "in_process": true,
		  "disclaimer": "aggregation only; not a public reserve attestation",
	   }
	   return d
   }
