package riskscore

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "riskscore_dump"

type dump struct {
	Events []Event `json:"events"`
	Hits   []Score `json:"hits,omitempty"`
	Halt bool `json:"halt,omitempty"`
	Mode Mode `json:"mode,omitempty"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		  return ErrEvent
	   }
	   s.mu.Lock()
	  d := dump{Events: append([]Event(nil),
		  s.events...), Hits: append([]Score(nil), s.hits...), Halt: s.halt, Mode: s.mode}
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
		   return persist.Replace(ctx,
			  kv, kindDump, key, body)
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
			  return nil, ErrEvent
		   }
		   return nil, err
	  }
	   var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	  }
	  s := New()
	   s.mu.Lock()
	  s.events = d.Events
	   s.hits = d.Hits
	   s.halt = d.Halt
	   if d.Mode != "" {
		   s.mode = d.Mode
	   }
	   s.mu.Unlock()
	   for _, e := range d.Events {
		  _, _ = s.graph.Add(e.Chain, e.Dest, "")
	   }
	   return s, nil
  }
