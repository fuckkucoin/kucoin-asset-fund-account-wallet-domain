// 领域服务：业务用例编排与校验。

// Author: EliotVoss
// Date: 2021.08.10

package ledbatch

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	rows map[string]*Batch
	byIdem map[string]string
	halt bool
	led *ledger.Service
	clock func() time.Time
}

   func New() *Service {
	   return &Service{
		  rows: map[string]*Batch{}, byIdem: map[string]string{},
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (s *Service) BindLedger(led *ledger.Service) *Service {
	  if s != nil {
		   s.led = led
	  }
	  return s
   }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock()
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

  func (s *Service) Get(id string) (Batch, error) {
	  if s == nil {
		  return Batch{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		   return Batch{}, ErrBatch
	  }
	  return cur.Copy(), nil
  }

  func requireActor(s string) (string, error) {
	   a, err := validate.Actor(s)
	   if err != nil {
		  return "", ErrActor
	  }
	   return a, nil
   }
