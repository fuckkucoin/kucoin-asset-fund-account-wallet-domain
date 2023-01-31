package sweeprun

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindSweepDump = "sweeprun_dump"

type dump struct {
	Jobs []Job `json:"jobs"`
	Budgets []Budget `json:"budgets"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrJob
	  }
	  s.mu.Lock()
	   d := dump{}
	   for _, j := range s.jobs {
		  d.Jobs = append(d.Jobs, j.Copy())
	   }
	   for _, b := range s.budget {
		  d.Budgets = append(d.Budgets, b.Copy())
	  }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	  if err != nil {
		   return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindSweepDump, key, body)
	   if err != nil {
		  return err
	  }
	  if existed {
		   return persist.Replace(ctx, kv, kindSweepDump, key, body)
	  }
	   return nil
   }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	  }
	  row, err := kv.Get(ctx, kindSweepDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			  return nil, ErrJob
		  }
		  return nil, err
	   }
	   var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	  s := New()
	   s.mu.Lock()
	  for i := range d.Jobs {
		   cp := d.Jobs[i]
		   s.jobs[cp.ID] = &cp
		  if cp.IdempotencyKey != "" {
			   s.byIdem[cp.IdempotencyKey] = cp.ID
		   }
	  }
	   for i := range d.Budgets {
		  cp := d.Budgets[i]
		  s.budget[budgetKey(cp.Chain, cp.Asset)] = &cp
	   }
	  s.mu.Unlock()
	   return s, nil
  }
