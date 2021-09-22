package p2pnet

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindP2PDump = "p2pnet_dump"

type dump struct {
	Rows    []Transfer `json:"rows"`
	Blocked []string   `json:"blocked"`
	Caps    Caps       `json:"caps"`
	WindowMS int64 `json:"window_ms"`
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
	  d := dump{Rows: s.dumpLocked(), Blocked: s.risk.Blocked(), Caps: s.vel.Caps(), WindowMS: s.window.Milliseconds()}
	   s.mu.Unlock()
	   body, err := json.Marshal(d)
	   if err != nil {
		   return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindP2PDump, key, body)
	  if err != nil {
		  return err
	   }
	  if existed {
		   return persist.Replace(ctx, kv, kindP2PDump, key, body)
	   }
	  return nil
   }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		  return nil, ErrUnavailable
	   }
	   row, err := kv.Get(ctx, kindP2PDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrNotFound
		  }
		   return nil, err
	  }
	  var d dump
	   if err := json.Unmarshal(row.Body,
		  &d); err != nil {
		   return nil, err
	   }
	  s := New()
	  for _, u := range d.Blocked {
		   if err := s.risk.Block(u); err != nil {
			   return nil, err
		  }
	  }
	   if err := s.vel.SetCaps(d.Caps); err != nil {
		   return nil, err
	   }
	   if d.WindowMS > 0 {
		   s.window = time.Duration(d.WindowMS) * time.Millisecond
	   }
	  s.mu.Lock()
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
