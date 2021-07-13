package hasync

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu    sync.Mutex
	halt  bool
	ready bool
	pair  *ha.Pair
	clock func() time.Time
	last []Probe
}

   func New() *Service {
	   return &Service{clock: func() time.Time { return time.Now().UTC() }}
   }

   func (s *Service) BindPair(p *ha.Pair) *Service {
	   if s != nil {
		   s.pair = p
	   }
	   return s
   }

   func (s *Service) SetHalt(on bool) {
	   if s == nil {
		   return
	  }
	  s.mu.Lock()
	  s.halt = on
	   s.mu.Unlock()
   }

  func (s *Service) Halted() bool {
	   if s == nil {
		   return false
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.halt
  }

   func (s *Service) SetReady(on bool) {
	  if s == nil {
		  return
	  }
	  s.mu.Lock()
	  s.ready = on
	  s.mu.Unlock()
   }

   func (s *Service) Ready() bool {
	  if s == nil {
		   return false
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.ready
  }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock()
  }

  func requireActor(s string) (string, error) {
	   a, err := validate.Actor(s)
	   if err != nil {
		   return "", ErrActor
	  }
	   return a, nil
  }

  func (s *Service) Pair() *ha.Pair {
	  if s == nil {
		   return nil
	   }
	  return s.pair
  }

  func (s *Service) History(limit int) []Probe {
	  if s == nil {
		   return nil
	  }
	   if limit <= 0 {
		  limit = 50
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if len(s.last) < limit {
		  return append([]Probe(nil), s.last...)
	   }
	  return append([]Probe(nil), s.last[len(s.last)-limit:]...)
  }

  func (s *Service) remember(p Probe) {
	   s.mu.Lock()
	  s.last = append(s.last, p)
	  if len(s.last) > 200 {
		   s.last = s.last[len(s.last)-200:]
	   }
	  s.mu.Unlock()
  }
