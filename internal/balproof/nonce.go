package balproof

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func (s *Service) takeNonce(nonce string) error {
	   n := strings.TrimSpace(nonce)
	  if n == "" || len(n) < 8 || len(n) > 128 {
		  return ErrNonce
	  }
	   for _, r := range n {
		   if r < 32 || r == '|' {
			   return ErrNonce
		   }
	  }
	  if _, err := validate.Idem(n); err != nil {
		  return ErrNonce
	  }
	   cut := s.now().Add(-s.ttl)
	  if used, ok := s.nonces[n]; ok {
		   if used.After(cut) {
			   return ErrReplay
		   }
	  }
	  s.nonces[n] = s.now()
	  for k, at := range s.nonces {
		  if at.Before(cut) {
			  delete(s.nonces, k)
		   }
	   }
	   return nil
   }

   func (s *Service) Used(limit int) []string {
	   if s == nil {
		   return nil
	  }
	   if limit <= 0 {
		  limit = 50
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]string, 0, limit)
	   for k := range s.nonces {
		   out = append(out, k)
		  if len(out) >= limit {
			  break
		  }
	  }
	   return out
   }
