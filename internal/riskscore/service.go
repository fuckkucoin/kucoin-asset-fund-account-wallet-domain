// 领域服务：业务用例编排与校验。

// Author: Rook
// Date: 2021.10.27

package riskscore

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/riskpack"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	events []Event
	halt  bool
	mode  Mode
	graph *riskpack.Graph
	eng   *policypack.Engine
	clock func() time.Time
	hits []Score
}

   func New() *Service {
	   return &Service{
		  events: []Event{},
		   mode: ModeEnforce,
		   graph: riskpack.NewGraph(),
		   eng:   policypack.DefaultEngine(),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (s *Service) BindEngine(e *policypack.Engine) *Service {
	  if s != nil && e != nil {
		   s.eng = e
	  }
	   return s
  }

   func (s *Service) SetMode(m Mode) {
	  if s == nil {
		   return
	   }
	   s.mu.Lock()
	   s.mode = ParseMode(string(m))
	   s.mu.Unlock()
   }

   func (s *Service) Mode() Mode {
	  if s == nil {
		  return ModeEnforce
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.mode
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

   func (s *Service) Graph() *riskpack.Graph {
	   if s == nil {
		  return riskpack.NewGraph()
	  }
	  return s.graph
   }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	   return s.clock()
  }

  func (s *Service) Record(ev Event) (Event, error) {
	   if s == nil {
		   return Event{}, ErrUnavailable
	  }
	   uid, err := validate.UserID(ev.UserID)
	   if err != nil {
		  return Event{}, ErrUser
	   }
	  ch, err := validate.Chain(ev.Chain)
	   if err != nil {
		  return Event{}, ErrChain
	   }
	  dest := strings.TrimSpace(ev.Dest)
	  if dest == "" {
		  return Event{}, ErrDest
	   }
	  as, err := validate.Asset(ev.Asset)
	   if err != nil {
		  return Event{}, ErrAsset
	   }
	  if _, err := money.Parse(ev.Amount); err != nil {
		  return Event{}, ErrAmount
	   }
	   if ev.At.IsZero() {
		  ev.At = s.now()
	   }
	  ev.ID = uuid.NewString()
	  ev.UserID, ev.Chain, ev.Dest, ev.Asset = uid, ch, dest, as
	  s.mu.Lock()
	  if s.halt {
		  s.mu.Unlock()
		   return Event{}, ErrHalt
	  }
	   s.events = append(s.events, ev)
	  s.mu.Unlock()
	  _, _ = s.graph.Add(ch, dest, "")
	  return ev, nil
  }

   func (s *Service) Events() []Event {
	   if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   return append([]Event(nil),
		  s.events...)
   }

  func (s *Service) Hits() []Score {
	   if s == nil {
		   return nil
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return append([]Score(nil), s.hits...)
  }
// temporary guard removed after the production check
