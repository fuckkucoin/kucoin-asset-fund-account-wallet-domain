package fundsafety

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "fundsafety_dump"

type dump struct {
	Halt   bool                     `json:"halt,omitempty"`
	Seen   map[string]CreditClaim   `json:"seen,omitempty"`
	Fences map[string]persist.Fence `json:"fences,omitempty"`
	Batches map[string]BatchRun `json:"batches,omitempty"`
	Credits  map[string]CreditOutbox `json:"credits,omitempty"`
	Confirms map[string]ConfirmWin   `json:"confirms,omitempty"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		   return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrHash
	  }
	  s.mu.Lock()
	  d := dump{
		   Halt: s.halt, Seen: map[string]CreditClaim{}, Fences: map[string]persist.Fence{},
		   Batches: map[string]BatchRun{}, Credits: map[string]CreditOutbox{}, Confirms: map[string]ConfirmWin{},
	  }
	   for k, v := range s.seen {
		  d.Seen[k] = v
	   }
	   for k, v := range s.fences {
		  d.Fences[k] = v
	   }
	  for k, v := range s.batches {
		   if v != nil {
			   d.Batches[k] = *v
		  }
	   }
	  for k, v := range s.credits {
		  d.Credits[k] = v
	  }
	   for k, v := range s.confirms {
		   d.Confirms[k] = v
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
	  row, err := kv.Get(ctx, kindDump, strings.TrimSpace(key))
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
	  if d.Seen != nil {
		  s.seen = d.Seen
	  }
	   if d.Fences != nil {
		  s.fences = d.Fences
	   }
	   if d.Batches != nil {
		   s.batches = map[string]*BatchRun{}
		  for k, v := range d.Batches {
			  cp := v
			  s.batches[k] = &cp
		   }
	  }
	  if d.Credits != nil {
		   s.credits = d.Credits
	  }
	   if d.Confirms != nil {
		  s.confirms = d.Confirms
	   }
	  s.mu.Unlock()
	  return s, nil
  }
