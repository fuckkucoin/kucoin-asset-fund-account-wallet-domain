package wdguard

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/addrintel"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/riskscore"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	halt bool
	pair *ha.Pair
	san  *addrintel.List
	eng   *policypack.Engine
	score *riskscore.Service
	kyt   kyt.Screener
	allow *destallow.Book
	clock func() time.Time
	last []Result
}

   func New() *Service {
	  return &Service{
		   san: addrintel.NewList(),
		   eng: policypack.DefaultEngine(),
		  score: riskscore.New(),
		   kyt:   kyt.Static{Mode: kyt.Allow},
		   allow: destallow.New(),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (s *Service) Book() *destallow.Book {
	   if s == nil || s.allow == nil {
		  return destallow.New()
	   }
	   return s.allow
  }

  func (s *Service) BindAllow(b *destallow.Book) *Service {
	   if s != nil && b != nil {
		   s.allow = b
	   }
	  return s
  }

   func (s *Service) BindPair(p *ha.Pair) *Service {
	   if s != nil {
		  s.pair = p
	   }
	  return s
   }

   func (s *Service) BindSanctions(l *addrintel.List) *Service {
	   if s != nil && l != nil {
		   s.san = l
	   }
	  return s
  }

  func (s *Service) BindEngine(e *policypack.Engine) *Service {
	  if s != nil && e != nil {
		  s.eng = e
	  }
	   return s
  }

  func (s *Service) BindScore(sc *riskscore.Service) *Service {
	  if s != nil && sc != nil {
		   s.score = sc
	   }
	   return s
   }

   func (s *Service) BindKYT(k kyt.Screener) *Service {
	  if s != nil && k != nil {
		   s.kyt = k
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

   func (s *Service) Recent(limit int) []Result {
	   if s == nil {
		   return nil
	   }
	   if limit <= 0 {
		  limit = 50
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if len(s.last) < limit {
		   return append([]Result(nil),
			  s.last...)
	   }
	   return append([]Result(nil), s.last[len(s.last)-limit:]...)
  }

   func requireActor(s string) (string, error) {
	  a, err := validate.Actor(s)
	  if err != nil {
		   return "", ErrActor
	  }
	   return a, nil
   }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	  return s.clock()
  }
