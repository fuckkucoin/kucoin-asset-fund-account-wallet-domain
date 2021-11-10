package safesaga

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "safesaga_dump"

type dump struct {
	Halt bool           `json:"halt,omitempty"`
	Runs map[string]Run `json:"runs,omitempty"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrIdem
	   }
	   s.mu.Lock()
	  d := dump{Halt: s.halt, Runs: map[string]Run{}}
	  for k, v := range s.runs {
		  d.Runs[k] = *v
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
			   return nil, ErrUnavailable
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
	  for k, v := range d.Runs {
		   cp := v
		   s.runs[k] = &cp
	  }
	  s.mu.Unlock()
	  return s, nil
  }
