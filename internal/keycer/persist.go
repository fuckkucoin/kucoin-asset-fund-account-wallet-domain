package keycer

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindKeyDump = "keycer_dump"

type dump struct {
	Slots []Slot `json:"slots"`
	Tickets []Ticket `json:"tickets"`
	Rots []Rotation `json:"rotations"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		   return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrSlot
	  }
	  s.mu.Lock()
	   d := dump{}
	  for _, r := range s.slots {
		   d.Slots = append(d.Slots, r.Copy())
	  }
	  for _, t := range s.tickets {
		  d.Tickets = append(d.Tickets,
			  t.Copy())
	  }
	   d.Rots = append(d.Rots, s.rots...)
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	  if err != nil {
		  return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindKeyDump, key, body)
	   if err != nil {
		  return err
	   }
	   if existed {
		  return persist.Replace(ctx,
			  kv, kindKeyDump, key, body)
	   }
	   return nil
   }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	  }
	   row,
		  err := kv.Get(ctx, kindKeyDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			  return nil, ErrSlot
		   }
		  return nil, err
	   }
	  var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	   s := New()
	   s.mu.Lock()
	  for i := range d.Slots {
		   cp := d.Slots[i]
		  s.slots[cp.ID] = &cp
	  }
	  for i := range d.Tickets {
		  cp := d.Tickets[i]
		   s.tickets[cp.ID] = &cp
	   }
	   s.rots = append(s.rots, d.Rots...)
	  s.mu.Unlock()
	   return s, nil
   }
