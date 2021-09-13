package oracle

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindOracleDump = "oracle_dump"

type dump struct {
	Ticks  []Tick        `json:"ticks"`
	MaxAge time.Duration `json:"max_age"`
}

   func (f *Feed) Save(ctx context.Context, kv persist.KV, key string) error {
	  if f == nil || kv == nil {
		  return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrPair
	   }
	   body, err := json.Marshal(dump{Ticks: f.List(), MaxAge: f.maxAge})
	   if err != nil {
		  return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindOracleDump, key, body)
	   if err != nil {
		   return err
	  }
	   if existed {
		   return persist.Replace(ctx,
			   kv, kindOracleDump, key, body)
	   }
	  return nil
   }

   func LoadFeed(ctx context.Context, kv persist.KV, key string) (*Feed, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	   }
	   row, err := kv.Get(ctx, kindOracleDump, strings.TrimSpace(key))
	   if err != nil {
		  if err == persist.ErrNotFound {
			  return nil, ErrMissing
		   }
		   return nil, err
	  }
	  var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	   f := NewFeed(d.MaxAge)
	   for _, t := range d.Ticks {
		   if err := f.Put(t); err != nil {
			   return nil, err
		  }
	   }
	  return f, nil
  }
