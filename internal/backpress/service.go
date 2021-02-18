// 领域服务：业务用例编排与校验。

// Author: Cason
// Date: 2021.02.17

package backpress

import (
	"strings"
	"sync"
	"sync/atomic"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Service struct {
	mu   sync.Mutex
	halt bool
	closed  bool
	cap     int
	workers int
	q []Job
	inflight int
	dropped int64
	done    int64
	fn func(Job) error
}

  func New(cap, workers int) *Service {
	   if cap <= 0 {
		   cap = 64
	  }
	   if workers <= 0 {
		  workers = 4
	   }
	   return &Service{cap: cap, workers: workers, q: []Job{}}
   }

   func (s *Service) On(fn func(Job) error) *Service {
	  if s != nil {
		   s.fn = fn
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

// Offer 锁内只入队。满则拒，不在锁里跑业务。
   func (s *Service) Offer(kind, payload string) (Job, error) {
	   if s == nil {
		   return Job{}, ErrUnavailable
	   }
	  kind = strings.TrimSpace(kind)
	   payload = strings.TrimSpace(payload)
	  if kind == "" || payload == "" {
		   return Job{}, ErrJob
	   }
	   s.mu.Lock()
	   if s.halt {
		  s.mu.Unlock()
		  return Job{}, ErrHalt
	  }
	   if s.closed {
		   s.mu.Unlock()
		   return Job{}, ErrClosed
	   }
	  if len(s.q) >= s.cap {
		   s.mu.Unlock()
		   atomic.AddInt64(&s.dropped, 1)
		  return Job{}, ErrFull
	  }
	   j := Job{ID: uuid.NewString(), Kind: kind, Payload: payload}
	  s.q = append(s.q, j)
	  s.mu.Unlock()
	  return j, nil
   }

// Drain 把队列拷出后放锁，再在锁外处理。
  func (s *Service) Drain(limit int) []Job {
	   if s == nil {
		  return nil
	   }
	   if limit <= 0 {
		   limit = 16
	   }
	   s.mu.Lock()
	  if len(s.q) == 0 {
		   s.mu.Unlock()
		  return nil
	   }
	   if limit > len(s.q) {
		  limit = len(s.q)
	   }
	   out := append([]Job{}, s.q[:limit]...)
	   s.q = s.q[limit:]
	  s.inflight += len(out)
	  s.mu.Unlock()
	   return out
   }

   func (s *Service) Finish(n int, errn int) {
	  if s == nil {
		   return
	  }
	  s.mu.Lock()
	   if n > s.inflight {
		  n = s.inflight
	  }
	   s.inflight -= n
	  s.mu.Unlock()
	  atomic.AddInt64(&s.done, int64(n-errn))
  }

// RunOnce 批次管线：拷队列 → 锁外执行 → 回写计数。

  func (s *Service) RunOnce(limit int) Batch {
	   jobs := s.Drain(limit)
	   b := Batch{ID: uuid.NewString(), Requested: len(jobs)}
	  if len(jobs) == 0 {
		  return b
	   }
	   fail := 0
	  for _, j := range jobs {
		   if s.fn == nil {
			  b.Accepted++
			   continue
		   }
		  if err := s.fn(j); err != nil {
			   fail++
			  b.Failed++
			   continue
		  }
		  b.Accepted++
	  }
	  s.Finish(len(jobs), fail)
	   return b
   }

   func (s *Service) Stats() Stats {
	  if s == nil {
		   return Stats{}
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   return Stats{
		   Queued: len(s.q), Cap: s.cap, Inflight: s.inflight, Workers: s.workers,
		   Dropped: atomic.LoadInt64(&s.dropped), Done: atomic.LoadInt64(&s.done),
		   Host: "query-gateway.kucoin.net",
	   }
  }

  func PoolNote(p persist.Profile) persist.PoolSettings {
	   return persist.PoolFor(p)
  }
