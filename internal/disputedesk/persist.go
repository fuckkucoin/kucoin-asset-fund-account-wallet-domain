package disputedesk

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDeskDump = "disputedesk_dump"

type dump struct {
	Cases    []Case     `json:"cases"`
	Evidence []Evidence `json:"evidence"`
	Events []Event `json:"events,omitempty"`
	Gens []CaseGeneration `json:"gens,omitempty"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrCase
	   }
	  s.mu.Lock()
	  d := dump{}
	   for _, c := range s.cases {
		   d.Cases = append(d.Cases, c.Copy())
	   }
	  for _, rows := range s.ev {
		  d.Evidence = append(d.Evidence, rows...)
	   }
	  d.Events = append(d.Events, s.events...)
	   for _, g := range s.gens {
		   cp := *g
		  d.Gens = append(d.Gens, cp)
	   }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	   if err != nil {
		  return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindDeskDump, key, body)
	  if err != nil {
		  return err
	   }
	  if existed {
		  return persist.Replace(ctx, kv, kindDeskDump, key, body)
	  }
	   return nil
  }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	  }
	   row, err := kv.Get(ctx, kindDeskDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrCase
		   }
		   return nil, err
	   }
	   var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	  }
	  s := New()
	  s.mu.Lock()
	  for i := range d.Cases {
		   cp := d.Cases[i]
		   s.cases[cp.ID] = &cp
		   if cp.IdempotencyKey != "" {
			   s.byIdem[cp.IdempotencyKey] = cp.ID
		  }
	   }
	   for _, e := range d.Evidence {
		  s.ev[e.CaseID] = append(s.ev[e.CaseID], e)
	  }
	   s.events = append(s.events, d.Events...)
	   for i := range d.Gens {
		   cp := d.Gens[i]
		   s.gens[cp.CaseID] = &cp
	  }
	   s.mu.Unlock()
	  return s, nil
   }
