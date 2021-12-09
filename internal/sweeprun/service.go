// @FileName: service.go
// @Summary: 领域服务：业务用例编排与校验。
// @Author: Jett
// @Created: 2021.12.08

package sweeprun

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	jobs map[string]*Job
	byIdem map[string]string
	budget map[string]*Budget
	led    *ledger.Service
	clock  func() time.Time
	agree  AgreeLeaseAsserter
	gate   *AgreeBook
}

   func New() *Service {
	  return &Service{
		  jobs: map[string]*Job{}, byIdem: map[string]string{},
		  budget: map[string]*Budget{},
		   clock:  func() time.Time { return time.Now().UTC() },
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

   func (s *Service) Enqueue(req EnqueueReq) (Job, error) {
	  if s == nil {
		   return Job{}, ErrUnavailable
	   }
	   ch, err := validate.Chain(req.Chain)
	   if err != nil {
		   return Job{}, ErrChain
	  }
	   asset, err := validate.Asset(req.Asset)
	  if err != nil {
		   return Job{}, ErrAsset
	  }
	  amt,
		   err := money.Parse(req.Amount)
	  if err != nil {
		  return Job{}, ErrAmount
	   }
	   idem,
		   err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		   return Job{}, ErrIdem
	  }
	   cap := amt
	   if strings.TrimSpace(req.JobCap) != "" {
		  cap, err = money.Parse(req.JobCap)
		  if err != nil {
			  return Job{}, ErrCap
		  }
		  if cap.Cmp(amt) < 0 {
			   return Job{}, ErrCap
		   }
	  }
	   from := first(req.From, "hot")
	   to := first(req.To, "cold")
	   if err := s.requirePlanLease(context.Background(), req, idem); err != nil {
		  return Job{}, err
	  }
	   s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.jobs[id]
		  s.mu.Unlock()
		   if cur.Chain != ch || cur.Asset != asset || cur.Amount != amt.String() {
			   return Job{}, ErrIdem
		   }
		   return cur.Copy(), nil
	   }
	   now := s.now()
	   job := &Job{
		   ID: uuid.NewString(), IdempotencyKey: idem, Chain: ch, Asset: asset,
		  Amount: amt.String(), Filled: "0", Remaining: amt.String(), JobCap: cap.String(),
		   State: StateQueued,
		   From:  from, To: to, CreatedAt: now, UpdatedAt: now,
		   AgreeLeaseID: strings.TrimSpace(req.AgreeLeaseID),
	  }
	   s.jobs[job.ID] = job
	  s.byIdem[idem] = job.ID
	  s.mu.Unlock()
	   return job.Copy(), nil
  }

  func (s *Service) EnqueueBatch(rows []EnqueueReq) ([]Job, error) {
	  out := make([]Job, 0, len(rows))
	  for _, r := range rows {
		  got, err := s.Enqueue(r)
		   if err != nil {
			   return out, err
		   }
		   out = append(out, got)
	   }
	   return out, nil
  }

  func (s *Service) Get(id string) (Job, error) {
	   if s == nil {
		   return Job{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cur,
		   ok := s.jobs[strings.TrimSpace(id)]
	   if !ok {
		  return Job{}, ErrJob
	   }
	   return cur.Copy(), nil
  }

   func (s *Service) List(chain string, limit int) []Job {
	  if s == nil {
		   return nil
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   if limit <= 0 {
		  limit = 50
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Job, 0, limit)
	   for _, j := range s.jobs {
		  if chain != "" && j.Chain != chain {
			   continue
		  }
		   out = append(out, j.Copy())
		  if len(out) >= limit {
			  break
		   }
	   }
	   return out
   }

   func (s *Service) ScanResume(limit int) []Job {
	  if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		  limit = 50
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Job, 0, limit)
	  for _, j := range s.jobs {
		  if !j.State.CanResume() {
			   continue
		   }
		  out = append(out, j.Copy())
		   if len(out) >= limit {
			  break
		   }
	  }
	  return out
   }

   func first(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		  return strings.TrimSpace(a)
	  }
	  return b
  }
