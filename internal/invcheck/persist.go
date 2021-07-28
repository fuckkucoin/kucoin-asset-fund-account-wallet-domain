package invcheck

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindInvDump = "invcheck_dump"

type dump struct {
	Reps []Report `json:"reps"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrNotFound
	   }
	  s.mu.Lock()
	  d := dump{}
	   for _, r := range s.reps {
		   d.Reps = append(d.Reps, r.Copy())
	   }
	   s.mu.Unlock()
	  body, err := json.Marshal(d)
	  if err != nil {
		   return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindInvDump, key, body)
	   if err != nil {
		   return err
	   }
	   if existed {
		  return persist.Replace(ctx, kv, kindInvDump, key, body)
	   }
	  return nil
   }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	  }
	  row, err := kv.Get(ctx, kindInvDump, strings.TrimSpace(key))
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
	   for i := range d.Reps {
		  cp := d.Reps[i]
		   s.reps[cp.ID] = &cp
	  }
	   s.mu.Unlock()
	   return s, nil
  }
