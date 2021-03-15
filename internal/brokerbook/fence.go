package brokerbook

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Fence 收紧子账权限。围栏打开后不得再划拨，直到 Unfence。

   func (s *Service) Fence(subID, actor string, privs []Priv) (Sub, error) {
	  if s == nil {
		  return Sub{}, ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		  return Sub{}, ErrActor
	  }
	  clean := make([]Priv, 0, len(privs))
	  seen := map[Priv]struct{}{}
	  for _, p := range privs {
		  got, err := ParsePriv(string(p))
		  if err != nil {
			  return Sub{}, err
		  }
		   if _, ok := seen[got]; ok {
			   continue
		  }
		   seen[got] = struct{}{}
		  clean = append(clean, got)
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.subs[strings.TrimSpace(subID)]
	   if !ok {
		  return Sub{}, ErrSub
	   }
	   cur.Privileges = clean
	  cur.Fenced = true
	  cur.UpdatedAt = s.now()
	  return cur.Copy(), nil
   }

   func (s *Service) Unfence(subID, actor string) (Sub, error) {
	  if s == nil {
		  return Sub{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return Sub{}, ErrActor
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.subs[strings.TrimSpace(subID)]
	  if !ok {
		   return Sub{}, ErrSub
	  }
	   cur.Fenced = false
	  cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
   }

  func (s *Service) HasPriv(subID string, want Priv) bool {
	   if s == nil {
		   return false
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.subs[strings.TrimSpace(subID)]
	   if !ok {
		  return false
	  }
	   if cur.Fenced && want != PrivTrade {
		  return false
	   }
	   return hasPrivLocked(cur.Copy(), want)
  }
