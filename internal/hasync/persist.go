package hasync

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "hasync_dump"

type dump struct {
	Halt  bool    `json:"halt,omitempty"`
	Ready bool    `json:"ready,omitempty"`
	Last  []Probe `json:"last,omitempty"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		  return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrActor
	  }
	  s.mu.Lock()
	   d := dump{Halt: s.halt, Ready: s.ready, Last: append([]Probe(nil), s.last...)}
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
	  s.ready = d.Ready
	  s.last = d.Last
	   s.mu.Unlock()
	   return s, nil
   }
