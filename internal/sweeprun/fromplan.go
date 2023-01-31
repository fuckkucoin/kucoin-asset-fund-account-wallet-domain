package sweeprun

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Plan struct {
	Chain string `json:"chain"`
	Asset        string `json:"asset"`
	Amount       string `json:"amount"`
	Idem         string `json:"idempotency_key,omitempty"`
	AgreeLeaseID string `json:"agree_lease_id,omitempty"`
}

// EnqueuePlans 把多链计划落成 queued job。金额必须过 Parse，失败即停。

  func (s *Service) EnqueuePlans(actor string, plans []Plan) ([]Job, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return nil, ErrActor
	   }
	   out := make([]Job, 0, len(plans))
	   for i, p := range plans {
		  idem := strings.TrimSpace(p.Idem)
		   if idem == "" {
			  idem = fmt.Sprintf("%s-%s-%d", p.Chain, p.Asset, i)
		  }
		   if _, err := money.Parse(p.Amount); err != nil {
			   return out, ErrAmount
		   }
		  got, err := s.Enqueue(EnqueueReq{
			   Chain: p.Chain, Asset: p.Asset, Amount: p.Amount, IdempotencyKey: idem,
			  AgreeLeaseID: p.AgreeLeaseID,
		  })
		   if err != nil {
			  return out, err
		   }
		   out = append(out, got)
	   }
	   return out, nil
   }

  func (s *Service) StartAll(actor string) (int, error) {
	  if s == nil {
		   return 0, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		  return 0, ErrActor
	  }
	   s.mu.Lock()
	  ids := make([]string, 0)
	   for _, j := range s.jobs {
		   if j.State == StateQueued {
			  ids = append(ids, j.ID)
		   }
	   }
	   s.mu.Unlock()
	  n := 0
	   for _, id := range ids {
		   if _, err := s.Start(id, actor); err != nil {
			   return n, err
		  }
		   n++
	  }
	  return n, nil
   }
