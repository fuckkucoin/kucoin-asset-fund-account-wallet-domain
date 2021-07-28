package invcheck

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu    sync.Mutex
	reps  map[string]*Report
	store ledger.Store
	cust *custody.Service
	tre *treasury.Service
	inv *hotinv.Book
	clock func() time.Time
}

   func New() *Service {
	   return &Service{
		   reps:  map[string]*Report{},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (s *Service) BindStore(st ledger.Store) *Service {
	  if s != nil {
		   s.store = st
	  }
	   return s
  }

  func (s *Service) BindCustody(c *custody.Service) *Service {
	   if s != nil {
		   s.cust = c
	   }
	  return s
  }

  func (s *Service) BindTreasury(t *treasury.Service) *Service {
	  if s != nil {
		  s.tre = t
	   }
	   return s
   }

  func (s *Service) BindHot(b *hotinv.Book) *Service {
	  if s != nil {
		   s.inv = b
	   }
	  return s
   }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock()
   }

   func (s *Service) Last() (Report, error) {
	  if s == nil {
		   return Report{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  var latest *Report
	   for _, r := range s.reps {
		   if latest == nil || r.At.After(latest.At) {
			   latest = r
		  }
	   }
	   if latest == nil {
		  return Report{}, ErrNotFound
	   }
	   return latest.Copy(), nil
  }

   func (s *Service) Get(id string) (Report, error) {
	   if s == nil {
		   return Report{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.reps[strings.TrimSpace(id)]
	  if !ok {
		  return Report{}, ErrNotFound
	  }
	  return cur.Copy(), nil
  }

  func (s *Service) List(limit int) []Report {
	   if s == nil {
		  return nil
	   }
	   if limit <= 0 {
		  limit = 50
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Report, 0, limit)
	   for _, r := range s.reps {
		   out = append(out, r.Copy())
		   if len(out) >= limit {
			  break
		   }
	   }
	  return out
  }

  func requireActor(s string) (string, error) {
	   a, err := validate.Actor(s)
	  if err != nil {
		  return "", ErrActor
	  }
	  return a, nil
  }
