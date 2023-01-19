package vipfee

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindVIPDump = "vipfee_dump"

type dump struct {
	Schedules []Schedule `json:"schedules"`
	Pins      []Pin      `json:"pins"`
	Fills []Fill            `json:"fills"`
	Promo map[string]string `json:"promo,omitempty"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	   }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrSchedule
	   }
	  s.mu.Lock()
	  d := dump{Promo: map[string]string{}}
	  for _, row := range s.rows {
		  d.Schedules = append(d.Schedules, row.Copy())
	   }
	   for _, p := range s.pins {
		   d.Pins = append(d.Pins, p.Copy())
	   }
	  d.Fills = append(d.Fills, s.fills...)
	   for t, v := range s.promo {
		  d.Promo[string(t)] = v
	   }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	   if err != nil {
		   return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindVIPDump, key, body)
	  if err != nil {
		   return err
	   }
	  if existed {
		  return persist.Replace(ctx, kv, kindVIPDump, key, body)
	  }
	  return nil
  }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	  }
	   row, err := kv.Get(ctx, kindVIPDump, strings.TrimSpace(key))
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
	  s.rows = map[string]Schedule{}
	   s.pins = map[string]Pin{}
	   s.fills = nil
	  for _, sch := range d.Schedules {
		  cp := sch
		   s.rows[scheduleKey(cp.Tier, cp.Kind, cp.Asset)] = cp
		  if cp.Version > s.seq {
			  s.seq = cp.Version
		  }
	   }
	   for _, p := range d.Pins {
		  cp := p
		   s.pins[cp.UserID] = cp
	   }
	   s.fills = append(s.fills, d.Fills...)
	   for k, v := range d.Promo {
		   if t, err := ParseTier(k); err == nil {
			  s.promo[t] = v
		   }
	   }
	   s.mu.Unlock()
	  return s, nil
   }
