package airgap

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindAirDump = "airgap_dump"

type dump struct {
	Sessions []Session `json:"sessions"`
	Envelopes []Envelope        `json:"envelopes"`
	Sealed    map[string][]byte `json:"sealed,omitempty"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		  return ErrSession
	  }
	  s.mu.Lock()
	   rows,
		  envs, sealed := s.dumpLocked()
	  s.mu.Unlock()
	  body,
		   err := json.Marshal(dump{Sessions: rows, Envelopes: envs, Sealed: sealed})
	  if err != nil {
		   return err
	  }
	   _,
		  existed, err := persist.GetOrInit(ctx, kv, kindAirDump, key, body)
	  if err != nil {
		   return err
	  }
	   if existed {
		   return persist.Replace(ctx, kv, kindAirDump, key, body)
	   }
	  return nil
  }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	   }
	  row,
		   err := kv.Get(ctx, kindAirDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrSession
		   }
		   return nil, err
	  }
	  var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	   }
	   s := New()
	   s.mu.Lock()
	   for i := range d.Sessions {
		   cp := d.Sessions[i]
		  s.rows[cp.ID] = &cp
	  }
	  for _, e := range d.Envelopes {
		   s.envelopes[e.SessionID] = e.Copy()
	  }
	  for k, v := range d.Sealed {
		  s.sealed[k] = append([]byte(nil), v...)
	   }
	  s.mu.Unlock()
	  return s, nil
  }
