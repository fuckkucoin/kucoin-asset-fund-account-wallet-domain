package ratelimit

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindRateDump = "ratelimit_dump"

type dump struct {
	Cap int `json:"cap"`
	Rate float64  `json:"rate"`
	Keys []string `json:"keys"`
}

  func (t *Table) Save(ctx context.Context, kv persist.KV, key string) error {
	  if t == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrActor
	  }
	   t.mu.Lock()
	   keys := make([]string,
		  0, len(t.rows))
	   for k := range t.rows {
		   keys = append(keys, k)
	   }
	   cap, rate := t.cap, t.rate
	  t.mu.Unlock()
	  body, err := json.Marshal(dump{Cap: cap, Rate: rate, Keys: keys})
	  if err != nil {
		  return err
	  }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindRateDump, key, body)
	  if err != nil {
		  return err
	  }
	  if existed {
		   return persist.Replace(ctx, kv, kindRateDump, key, body)
	  }
	  return nil
   }

  func LoadTable(ctx context.Context, kv persist.KV, key string) (*Table, error) {
	  if kv == nil {
		  return nil, ErrUnavailable
	   }
	  row, err := kv.Get(ctx, kindRateDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			  return nil, ErrActor
		  }
		   return nil, err
	   }
	   var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	   }
	   tab := NewTable(d.Cap, d.Rate)
	   for _, k := range d.Keys {
		  _ = tab.Allow(k)
	   }
	   return tab, nil
  }
