package fiatrail

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindRailDump = "fiatrail_dump"

type dump struct {
	Bookings []Booking `json:"bookings"`
	Profiles []Profile `json:"profiles"`
	Intents  []Intent  `json:"intents"`
	Caps Caps     `json:"caps"`
	Bins []string `json:"bins"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrBooking
	   }
	  s.mu.Lock()
	  d := dump{Bookings: s.dumpLocked(), Profiles: s.kyc.List(), Intents: s.psp.List(), Caps: s.limits.Caps()}
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	   if err != nil {
		   return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindRailDump, key, body)
	   if err != nil {
		   return err
	   }
	  if existed {
		  return persist.Replace(ctx, kv, kindRailDump, key, body)
	  }
	  return nil
  }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	   }
	  row, err := kv.Get(ctx, kindRailDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrBooking
		  }
		   return nil, err
	   }
	  var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	  }
	  s := New()
	  for _, p := range d.Profiles {
		   if err := s.kyc.Put(p); err != nil {
			  return nil, err
		  }
	   }
	   if err := s.limits.SetCaps(d.Caps); err != nil {
		   return nil, err
	  }
	   for _, bin := range d.Bins {
		   if err := s.limits.BanBIN(bin); err != nil {
			  return nil, err
		  }
	   }
	   s.mu.Lock()
	  for i := range d.Bookings {
		   b := d.Bookings[i]
		   cp := b
		   s.rows[cp.ID] = &cp
		  if cp.IdempotencyKey != "" {
			   s.byIdem[cp.IdempotencyKey] = cp.ID
		  }
	   }
	  s.mu.Unlock()
	   return s, nil
   }
