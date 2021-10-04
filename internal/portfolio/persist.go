package portfolio

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindPortfolioDump = "portfolio_dump"
	kindPortfolioHop  = "portfolio_hop"
)

type Dump struct {
	Positions []Position `json:"positions"`
	Hops []HopRec `json:"hops"`
}

// Save 把影子子账和 hop 流水落到 KV。失败不得当内存账已丢。

  func (l *BookLedger) Save(ctx context.Context, kv persist.KV, key string) error {
	  if l == nil || kv == nil {
		   return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrUser
	   }
	  dump := Dump{Positions: l.List(""), Hops: l.Journal("")}
	   body, err := json.Marshal(dump)
	  if err != nil {
		  return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindPortfolioDump, key, body)
	  if err != nil {
		   return err
	  }
	   if existed {
		   return persist.Replace(ctx,
			  kv, kindPortfolioDump, key, body)
	  }
	   return nil
  }

  func Load(ctx context.Context, kv persist.KV, key string) (*BookLedger, error) {
	  if kv == nil {
		  return nil, ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	   if key == "" {
		   return nil, ErrUser
	   }
	  row, err := kv.Get(ctx, kindPortfolioDump, key)
	   if err != nil {
		   if err == persist.ErrNotFound {
			  return nil, ErrNotFound
		   }
		   return nil, err
	   }
	   var dump Dump
	   if err := json.Unmarshal(row.Body, &dump); err != nil {
		  return nil, err
	  }
	  l := NewBookLedger()
	  for _, p := range dump.Positions {
		   if _, err := l.Seed(p.UserID, string(p.Book), p.Asset, p.Available, p.Frozen); err != nil {
			   return nil, err
		  }
		  if p.Locked {
			   _ = l.SetLock(p.UserID, p.Book, p.Asset, true)
		  }
		   if p.Reserved {
			  _ = l.SetReserve(p.UserID, p.Book, p.Asset, true)
		   }
	   }
	   l.mu.Lock()
	  for i := range dump.Hops {
		  cp := dump.Hops[i]
		  l.journal[cp.Key] = &cp
	  }
	  l.mu.Unlock()
	  return l, nil
  }

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  return s.Ledger().Save(ctx, kv, key)
  }
