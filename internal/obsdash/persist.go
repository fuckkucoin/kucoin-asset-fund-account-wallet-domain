package obsdash

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDashDump = "obsdash_dump"

type dump struct {
	Points []Point `json:"points"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		  return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		  return ErrName
	  }
	  body, err := json.Marshal(dump{Points: s.ctr.Snapshot()})
	   if err != nil {
		  return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindDashDump, key, body)
	   if err != nil {
		   return err
	  }
	  if existed {
		  return persist.Replace(ctx, kv, kindDashDump, key, body)
	  }
	  return nil
  }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	  }
	   row, err := kv.Get(ctx, kindDashDump, strings.TrimSpace(key))
	  if err != nil {
		  if err == persist.ErrNotFound {
			   return nil, ErrName
		  }
		  return nil, err
	   }
	   var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	  s := New()
	  for _, p := range d.Points {
		  _ = s.ctr.Inc(p.Name, p.Label, p.Value)
	  }
	  return s, nil
  }
