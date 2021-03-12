package cfgcenter

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindCfgDump = "cfgcenter_dump"

type dump struct {
	Packs map[string][]LimitPack `json:"packs"`
	Pin map[string]int `json:"pin"`
	Kill map[string]bool `json:"kill"`
	Global bool `json:"global"`
}

  func (s *Store) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		  return ErrName
	  }
	  s.mu.Lock()
	  d := dump{
		  Packs:  map[string][]LimitPack{},
		   Pin:    map[string]int{},
		  Kill:   map[string]bool{},
		  Global: s.global,
	  }
	   for n, rows := range s.byName {
		  cp := make([]LimitPack, len(rows))
		  copy(cp, rows)
		  d.Packs[n] = cp
	   }
	   for n, v := range s.pin {
		   d.Pin[n] = v
	  }
	   for n, v := range s.kill {
		   d.Kill[n] = v
	   }
	  s.mu.Unlock()
	   body, err := json.Marshal(d)
	   if err != nil {
		  return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindCfgDump, key, body)
	  if err != nil {
		   return err
	  }
	   if existed {
		  return persist.Replace(ctx, kv, kindCfgDump, key, body)
	   }
	  return nil
   }

  func LoadStore(ctx context.Context, kv persist.KV, key string) (*Store, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	  }
	   row, err := kv.Get(ctx, kindCfgDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrName
		  }
		  return nil, err
	   }
	  var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	  s := &Store{
		  byName: map[string][]LimitPack{}, pin: map[string]int{}, kill: map[string]bool{},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
	   if d.Packs != nil {
		   s.byName = d.Packs
	  }
	  if d.Pin != nil {
		   s.pin = d.Pin
	   }
	  if d.Kill != nil {
		   s.kill = d.Kill
	   }
	   s.global = d.Global
	  return s, nil
   }
