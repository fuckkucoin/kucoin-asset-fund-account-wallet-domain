package balproof

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindProofDump = "balproof_dump"

type dump struct {
	Secret string  `json:"-"`
	Proofs []Proof `json:"proofs"`
	Nonces map[string]string `json:"nonces"`
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
	   d := dump{Nonces: map[string]string{}}
	   for _, p := range s.proofs {
		  d.Proofs = append(d.Proofs, p.Copy())
	   }
	   for n, at := range s.nonces {
		   d.Nonces[n] = at.UTC().Format(time.RFC3339Nano)
	  }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	   if err != nil {
		  return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindProofDump, key, body)
	   if err != nil {
		  return err
	  }
	  if existed {
		  return persist.Replace(ctx, kv, kindProofDump, key, body)
	  }
	   return nil
  }

  func Load(ctx context.Context, kv persist.KV, key, secret string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	  }
	   row,
		  err := kv.Get(ctx, kindProofDump, strings.TrimSpace(key))
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
	  s := New(secret)
	  s.mu.Lock()
	  for _, p := range d.Proofs {
		   s.proofs[p.ID] = p
	  }
	  for n, raw := range d.Nonces {
		  at, err := time.Parse(time.RFC3339Nano, raw)
		  if err != nil {
			   continue
		   }
		   s.nonces[n] = at
	  }
	  s.mu.Unlock()
	   return s, nil
  }
