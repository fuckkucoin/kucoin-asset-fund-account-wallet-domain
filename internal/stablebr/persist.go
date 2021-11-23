package stablebr

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindStabDump = "stablebr_dump"

type dump struct {
	Checks []storedCheck `json:"checks"`
	Rows    []Journal        `json:"rows"`
	MintGen map[string]int64 `json:"mint_gen,omitempty"`
}

type storedCheck struct {
	Key string `json:"key"`
	Check Check `json:"check"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	  if key == "" {
		  return ErrNotFound
	   }
	   s.mu.Lock()
	   d := dump{}
	  for k, c := range s.checks {
		   d.Checks = append(d.Checks, storedCheck{Key: k, Check: c.Copy()})
	   }
	   for _, r := range s.rows {
		  d.Rows = append(d.Rows, r.Copy())
	   }
	   if len(s.mintGen) > 0 {
		   d.MintGen = make(map[string]int64, len(s.mintGen))
		  for k, v := range s.mintGen {
			  d.MintGen[k] = v
		  }
	  }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	   if err != nil {
		  return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindStabDump, key, body)
	  if err != nil {
		   return err
	  }
	   if existed {
		   return persist.Replace(ctx, kv, kindStabDump, key, body)
	   }
	  return nil
   }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	   }
	  row, err := kv.Get(ctx, kindStabDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrNotFound
		   }
		   return nil, err
	   }
	   var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	   }
	  s := New()
	  s.mu.Lock()
	   for _, c := range d.Checks {
		   s.checks[c.Key] = c.Check
	   }
	  for i := range d.Rows {
		   cp := d.Rows[i]
		   s.rows[cp.ID] = &cp
		  if cp.IdempotencyKey != "" {
			   s.byIdem[cp.IdempotencyKey] = cp.ID
		   }
	   }
	   for k, v := range d.MintGen {
		   s.mintGen[k] = v
	  }
	  s.mu.Unlock()
	   return s, nil
  }
