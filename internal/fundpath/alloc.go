package fundpath

import (
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func (s *Service) Allocate(userID, chain string) (addrpool.Binding, error) {
	  if s == nil || s.pool == nil {
		   return addrpool.Binding{}, ErrAlloc
	   }
	   if s.Halted() {
		   return addrpool.Binding{}, ErrHalt
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return addrpool.Binding{}, ErrUser
	   }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		   return addrpool.Binding{}, ErrChain
	   }
	   b, err := s.pool.Allocate(uid, ch)
	   if err != nil {
		  return addrpool.Binding{}, err
	  }
	   return b, nil
   }

   func (s *Service) LookupAddr(userID, chain string) (addrpool.Binding, bool) {
	  if s == nil || s.pool == nil {
		   return addrpool.Binding{}, false
	   }
	   return s.pool.Lookup(userID, chain)
  }

   func (s *Service) Release(userID, chain string) error {
	  if s == nil || s.pool == nil {
		  return ErrAlloc
	  }
	   return s.pool.Release(userID, chain)
   }

   func (s *Service) Prefill(chain string, n int, seed string) (int, error) {
	   if s == nil || s.inv == nil {
		  return 0, ErrAlloc
	  }
	  return s.inv.Prefill(chain, n, seed)
  }
