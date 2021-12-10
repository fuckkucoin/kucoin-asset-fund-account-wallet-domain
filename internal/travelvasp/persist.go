package travelvasp

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindTravelDump = "travelvasp_dump"

type dump struct {
	VASPs []VASP `json:"vasps"`
	Msgs []Message `json:"messages"`
	Cases []Case `json:"cases"`
	Head  string `json:"head"`
	Chain []string `json:"chain"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	   }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrMessage
	  }
	  s.mu.Lock()
	  d := dump{Head: s.chainHead,
		   Chain: append([]string(nil), s.chain...)}
	  for _, v := range s.vasps {
		   d.VASPs = append(d.VASPs,
			   v.Copy())
	   }
	  for _, m := range s.msgs {
		   d.Msgs = append(d.Msgs, m.Copy())
	   }
	  for _, c := range s.cases {
		  d.Cases = append(d.Cases, c.Copy())
	   }
	  s.mu.Unlock()
	   body, err := json.Marshal(d)
	   if err != nil {
		  return err
	  }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindTravelDump, key, body)
	   if err != nil {
		   return err
	  }
	   if existed {
		  return persist.Replace(ctx, kv, kindTravelDump, key, body)
	   }
	  return nil
  }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	   }
	   row, err := kv.Get(ctx, kindTravelDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			  return nil, ErrMessage
		  }
		   return nil, err
	   }
	  var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	   s := New()
	  s.mu.Lock()
	  s.chainHead = d.Head
	  s.chain = append([]string(nil), d.Chain...)
	   for i := range d.VASPs {
		   cp := d.VASPs[i]
		   s.vasps[cp.Code] = &cp
		   s.indexLocked(&cp)
	  }
	   for i := range d.Msgs {
		  cp := d.Msgs[i]
		  s.msgs[cp.ID] = &cp
		   if cp.IdempotencyKey != "" {
			   s.byIdem[cp.IdempotencyKey] = cp.ID
		   }
	  }
	  for i := range d.Cases {
		   cp := d.Cases[i]
		   s.cases[cp.ID] = &cp
		   if cp.MovementID != "" {
			   s.byMove[cp.MovementID] = cp.ID
		  }
	   }
	   s.mu.Unlock()
	   return s, nil
   }
