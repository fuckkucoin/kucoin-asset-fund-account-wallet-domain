package brokerbook

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindBrokerDump = "brokerbook_dump"

type dump struct {
	Masters []Master `json:"masters"`
	Subs  []Sub  `json:"subs"`
	Books []Book `json:"books"`
	Allocs []Alloc `json:"allocs"`
	Fills  []Fill  `json:"fills"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrMaster
	   }
	  s.mu.Lock()
	   d := dump{}
	  for _, m := range s.masters {
		  d.Masters = append(d.Masters, m.Copy())
	   }
	  for _, r := range s.subs {
		  d.Subs = append(d.Subs, r.Copy())
	   }
	   for _, b := range s.books {
		   d.Books = append(d.Books,
			   b.Copy())
	  }
	   for _, a := range s.allocs {
		  d.Allocs = append(d.Allocs, a.Copy())
	   }
	   d.Fills = append(d.Fills, s.fills...)
	  s.mu.Unlock()
	   body, err := json.Marshal(d)
	   if err != nil {
		  return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindBrokerDump, key, body)
	   if err != nil {
		  return err
	   }
	   if existed {
		   return persist.Replace(ctx, kv, kindBrokerDump, key, body)
	  }
	   return nil
  }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	   }
	   row, err := kv.Get(ctx, kindBrokerDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			  return nil, ErrMaster
		   }
		   return nil, err
	   }
	  var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	   }
	   s := New()
	   s.mu.Lock()
	   for i := range d.Masters {
		  cp := d.Masters[i]
		  s.masters[cp.ID] = &cp
	   }
	  for i := range d.Subs {
		   cp := d.Subs[i]
		   s.subs[cp.ID] = &cp
	  }
	   for i := range d.Books {
		   cp := d.Books[i]
		  s.books[bookKey(cp.OwnerID, cp.Asset)] = &cp
	  }
	  for i := range d.Allocs {
		  cp := d.Allocs[i]
		   s.allocs[cp.ID] = &cp
		  if cp.IdempotencyKey != "" {
			  s.byIdem[cp.IdempotencyKey] = cp.ID
		  }
	   }
	  s.fills = append(s.fills,
		   d.Fills...)
	   s.mu.Unlock()
	  return s, nil
  }
