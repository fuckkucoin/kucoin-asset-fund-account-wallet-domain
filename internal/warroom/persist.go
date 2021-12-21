package warroom

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindWarDump = "warroom_dump"

type dump struct {
	Switch  Switch   `json:"switch"`
	Tickets []Ticket `json:"tickets"`
	Events  []Event  `json:"events"`
	Frozen  []string `json:"frozen"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrTicket
	   }
	   s.mu.Lock()
	   d := dump{Switch: s.sw.Copy()}
	   for _, t := range s.tickets {
		  d.Tickets = append(d.Tickets, t.Copy())
	   }
	  d.Events = append(d.Events, s.events...)
	   for u, on := range s.frozen {
		   if on {
			   d.Frozen = append(d.Frozen, u)
		  }
	   }
	  s.mu.Unlock()
	   body, err := json.Marshal(d)
	   if err != nil {
		   return err
	   }
	   _,
		  existed, err := persist.GetOrInit(ctx, kv, kindWarDump, key, body)
	   if err != nil {
		  return err
	   }
	   if existed {
		  return persist.Replace(ctx, kv, kindWarDump, key, body)
	  }
	   return nil
   }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	  }
	  row, err := kv.Get(ctx, kindWarDump, strings.TrimSpace(key))
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
	   s.sw = d.Switch
	   for i := range d.Tickets {
		  cp := d.Tickets[i]
		  s.tickets[cp.ID] = &cp
	  }
	   s.events = append(s.events, d.Events...)
	   for _, u := range d.Frozen {
		   s.frozen[u] = true
	  }
	   s.mu.Unlock()
	  return s, nil
   }
