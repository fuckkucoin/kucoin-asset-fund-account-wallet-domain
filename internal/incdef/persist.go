package incdef

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "incdef_dump"

type dump struct {
	Mode   Mode `json:"mode"`
	Killed bool `json:"killed,omitempty"`
	Frozen bool `json:"frozen,omitempty"`
	Degrade bool `json:"degrade,omitempty"`
	Seq    int64   `json:"seq,omitempty"`
	Events []Event `json:"events,omitempty"`
	Alerts []Alert `json:"alerts,omitempty"`
	Stuck map[string]StuckCase `json:"stuck,omitempty"`
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
	   stuck := map[string]StuckCase{}
	  for k, v := range s.stuck {
		  stuck[k] = v
	  }
	  d := dump{Mode: s.mode,
		   Killed: s.killed, Frozen: s.frozen, Degrade: s.degrade, Seq: s.seq,
		   Events: append([]Event{}, s.events...), Alerts: append([]Alert{}, s.alerts...), Stuck: stuck}
	   s.mu.Unlock()
	  body, err := json.Marshal(d)
	  if err != nil {
		   return err
	   }
	   _,
		   existed, err := persist.GetOrInit(ctx, kv, kindDump, key, body)
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
	   s.mode = d.Mode
	   s.killed = d.Killed
	   s.frozen = d.Frozen
	   s.degrade = d.Degrade
	   s.seq = d.Seq
	  if d.Events != nil {
		  s.events = d.Events
	  }
	   if d.Alerts != nil {
		   s.alerts = d.Alerts
	  }
	  if d.Stuck != nil {
		   s.stuck = d.Stuck
	   }
	  s.mu.Unlock()
	  return s, nil
   }
// temporary guard removed after the production check
