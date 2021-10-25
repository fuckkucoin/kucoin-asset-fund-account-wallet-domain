package riskgate

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/policypack"
)

   func policypackEngine(mode string) *policypack.Engine {
	  return policypack.NewEngine(policypack.NewCatalog(), policypack.ParseMode(mode), true)
  }

const kindDump = "riskgate_dump"

type dump struct {
	Halt bool     `json:"halt,omitempty"`
	Wins []Window `json:"wins,omitempty"`
	Edges map[string]map[string]int  `json:"edges,omitempty"`
	First map[string]map[string]bool `json:"first,omitempty"`
	Mode  string                     `json:"mode,omitempty"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		   return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrPack
	   }
	  s.mu.Lock()
	  d := dump{Halt: s.halt, Wins: append([]Window{}, s.wins...), Edges: s.edges, First: s.first}
	   if s.eng != nil {
		   d.Mode = string(s.eng.Mode())
	   }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	  if err != nil {
		  return err
	  }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindDump, key, body)
	  if err != nil {
		   return err
	  }
	  if existed {
		   return persist.Replace(ctx, kv, kindDump, key, body)
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
	  if d.Mode != "" {
		   s.eng = policypackEngine(d.Mode)
	  }
	   s.mu.Lock()
	  s.halt = d.Halt
	  if d.Wins != nil {
		   s.wins = d.Wins
	  }
	   if d.Edges != nil {
		  s.edges = d.Edges
	  }
	  if d.First != nil {
		   s.first = d.First
	  }
	  s.mu.Unlock()
	   return s, nil
   }
