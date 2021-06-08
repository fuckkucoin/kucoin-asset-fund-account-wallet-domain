package fundguard

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "fundguard_dump"

type dump struct {
	Halt  bool                 `json:"halt,omitempty"`
	Seen  map[string]HashClaim `json:"seen,omitempty"`
	Snaps []Bundle             `json:"snaps,omitempty"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrSnap
	  }
	   s.mu.Lock()
	  d := dump{Halt: s.halt, Seen: map[string]HashClaim{}, Snaps: make([]Bundle, 0, len(s.snaps))}
	   for k, v := range s.seen {
		  d.Seen[k] = v
	  }
	  for _, b := range s.snaps {
		   d.Snaps = append(d.Snaps, b.Copy())
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
			  return nil, ErrSnap
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
	   s.snaps = d.Snaps
	  s.mu.Unlock()
	   return s, nil
  }
