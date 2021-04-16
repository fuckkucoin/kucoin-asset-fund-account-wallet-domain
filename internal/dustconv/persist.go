package dustconv

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDustDump = "dustconv_dump"

type dump struct {
	Policies []Policy `json:"policies"`
	Rates []Rate `json:"rates"`
	Books []Book `json:"books"`
	Rows []Convert `json:"rows"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrUser
	   }
	  s.mu.Lock()
	   d := dump{}
	  for _, p := range s.policy {
		  d.Policies = append(d.Policies, p.Copy())
	  }
	  for _, r := range s.rates {
		  d.Rates = append(d.Rates, r.Copy())
	  }
	  for _, b := range s.books {
		   d.Books = append(d.Books, b.Copy())
	  }
	  for _, r := range s.rows {
		  d.Rows = append(d.Rows, r.Copy())
	   }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	  if err != nil {
		   return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindDustDump, key, body)
	   if err != nil {
		  return err
	  }
	   if existed {
		   return persist.Replace(ctx, kv, kindDustDump, key, body)
	   }
	   return nil
   }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		  return nil, ErrUnavailable
	  }
	  row, err := kv.Get(ctx, kindDustDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrUser
		  }
		  return nil, err
	  }
	   var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		  return nil, err
	  }
	  s := &Service{
		   policy: map[string]*Policy{}, rates: map[string]*Rate{},
		   books: map[string]*Book{}, rows: map[string]*Convert{},
		  byIdem:      map[string]string{},
		  genCounters: map[string]*genCounter{},
		  policyGen: map[string]int64{}, capBPS: map[string]int64{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
	   s.mu.Lock()
	   for i := range d.Policies {
		   cp := d.Policies[i]
		   s.policy[cp.Asset] = &cp
	   }
	   for i := range d.Rates {
		   cp := d.Rates[i]
		   s.rates[cp.Asset] = &cp
	   }
	  for i := range d.Books {
		   cp := d.Books[i]
		  s.books[bookKey(cp.UserID, cp.Asset)] = &cp
	  }
	  for i := range d.Rows {
		   cp := d.Rows[i]
		   s.rows[cp.ID] = &cp
		  if cp.IdempotencyKey != "" {
			   s.byIdem[cp.IdempotencyKey] = cp.ID
		  }
	  }
	  s.mu.Unlock()
	  return s, nil
   }
