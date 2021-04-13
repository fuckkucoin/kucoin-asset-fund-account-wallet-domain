package depbatch

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "depbatch_dump"

type dump struct {
	Rows []Batch           `json:"rows"`
	Seen map[string]string `json:"seen,omitempty"`
	Halt bool `json:"halt,omitempty"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrBatch
	  }
	  s.mu.Lock()
	  d := dump{Halt: s.halt, Seen: map[string]string{}}
	   for k, v := range s.seen {
		   d.Seen[k] = v
	  }
	   for _, r := range s.rows {
		   d.Rows = append(d.Rows, r.Copy())
	  }
	   s.mu.Unlock()
	   body, err := json.Marshal(d)
	  if err != nil {
		   return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindDump, key, body)
	  if err != nil {
		   return err
	  }
	  if existed {
		  return persist.Replace(ctx, kv, kindDump, key, body)
	   }
	  return nil
  }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	   }
	   row,
		   err := kv.Get(ctx, kindDump, strings.TrimSpace(key))
	  if err != nil {
		  if err == persist.ErrNotFound {
			   return nil, ErrBatch
		  }
		   return nil, err
	  }
	  var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	   }
	   s := New()
	   s.mu.Lock()
	  s.halt = d.Halt
	   if d.Seen != nil {
		  s.seen = d.Seen
	  }
	  for i := range d.Rows {
		  cp := d.Rows[i]
		   s.rows[cp.ID] = &cp
		   if cp.IdempotencyKey != "" {
			  s.byIdem[cp.IdempotencyKey] = cp.ID
		  }
	  }
	   s.mu.Unlock()
	   return s, nil
   }
