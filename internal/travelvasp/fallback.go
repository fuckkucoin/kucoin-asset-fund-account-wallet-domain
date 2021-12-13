package travelvasp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindTravelFallback = "travel.vasp.fallback"

// FallbackList 走廊或对手码的多跳回落。

type FallbackList struct {
	Key string `json:"key"`
	Hops []string `json:"hops"`
}

// FallbackBook 多跳回落表。
type FallbackBook struct {
	mu sync.RWMutex
	rows map[string]FallbackList
	kv persist.KV
}

   func NewFallbackBook() *FallbackBook {
	  return &FallbackBook{rows: map[string]FallbackList{}}
  }

  func (b *FallbackBook) BindKV(kv persist.KV) *FallbackBook {
	  if b != nil && kv != nil {
		   b.kv = kv
	  }
	  return b
  }

  func fallbackKey(key string) string {
	  k := strings.ToUpper(strings.TrimSpace(key))
	  if k == "" {
		   k = "DEFAULT"
	  }
	  return k
   }

  func (b *FallbackBook) Put(ctx context.Context, row FallbackList) error {
	  if b == nil {
		   return ErrUnavailable
	   }
	   row.Key = fallbackKey(row.Key)
	  clean := make([]string, 0, len(row.Hops))
	  seen := map[string]struct{}{}
	  for _, h := range row.Hops {
		  code := strings.ToUpper(strings.TrimSpace(h))
		  if code == "" {
			  continue
		   }
		   if _, ok := seen[code]; ok {
			  continue
		   }
		  seen[code] = struct{}{}
		  clean = append(clean, code)
	   }
	   if len(clean) == 0 {
		  return ErrPeer
	  }
	  row.Hops = clean
	  b.mu.Lock()
	  if b.rows == nil {
		   b.rows = map[string]FallbackList{}
	   }
	   b.rows[row.Key] = row
	  b.mu.Unlock()
	  if b.kv == nil {
		   return nil
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   body, err := json.Marshal(row)
	   if err != nil {
		  return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, b.kv, kindTravelFallback, row.Key, body)
	  if err != nil {
		  return err
	   }
	  if existed {
		   return persist.Replace(ctx,
			  b.kv, kindTravelFallback, row.Key, body)
	   }
	  return nil
  }

  func (b *FallbackBook) Hops(key string) []string {
	   if b == nil {
		  return nil
	   }
	   b.mu.RLock()
	   defer b.mu.RUnlock()
	  if r, ok := b.rows[fallbackKey(key)]; ok {
		  return append([]string(nil), r.Hops...)
	  }
	  if r, ok := b.rows["DEFAULT"]; ok {
		  return append([]string(nil),
			  r.Hops...)
	  }
	   return nil
   }

   func (b *FallbackBook) List() []FallbackList {
	   if b == nil {
		   return nil
	   }
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	  out := make([]FallbackList, 0, len(b.rows))
	  for _, r := range b.rows {
		  out = append(out, r)
	   }
	  return out
   }

   func (b *FallbackBook) Reload(ctx context.Context) error {
	   if b == nil || b.kv == nil {
		  return nil
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   rows, err := b.kv.List(ctx, kindTravelFallback, 400)
	   if err != nil {
		   return err
	   }
	   next := map[string]FallbackList{}
	  for _, row := range rows {
		   var r FallbackList
		   if json.Unmarshal(row.Body, &r) != nil {
			   continue
		   }
		  r.Key = fallbackKey(r.Key)
		  next[r.Key] = r
	   }
	   b.mu.Lock()
	  b.rows = next
	  b.mu.Unlock()
	  return nil
   }

   func (s *Service) BindFallback(book *FallbackBook) *Service {
	   if s != nil && book != nil {
		  s.fallback = book
	   }
	  return s
   }

   func (s *Service) Fallbacks() *FallbackBook {
	  if s == nil {
		   return nil
	   }
	   return s.fallback
  }

// ResolveHop 主查询失败后按 hop 精确回落。全部失败关闭。

  func (s *Service) ResolveHop(q ResolveQuery, hops []string) (VASP, error) {
	   if v, err := s.Resolve(q); err == nil && v.Enabled {
		  return v, nil
	  } else if err == nil && !v.Enabled {
		   // 主对手停用，接着 hop。
	   }
	  for _, hop := range hops {
		   v, err := s.RequireEnabled(strings.ToUpper(strings.TrimSpace(hop)))
		   if err != nil {
			   continue
		   }
		  return v, nil
	  }
	   return VASP{}, ErrPeer
  }

// PreferProtocol 按走廊协议序挑可用协议。空序回落单字段。
   func PreferProtocol(pol CorridorPolicy, peerProto string) (string, bool) {
	   order := pol.Protocols
	   if len(order) == 0 && strings.TrimSpace(pol.Protocol) != "" {
		  order = []string{pol.Protocol}
	  }
	   if len(order) == 0 {
		  return strings.ToUpper(strings.TrimSpace(peerProto)), true
	  }
	  peer := strings.ToUpper(strings.TrimSpace(peerProto))
	  for _, p := range order {
		  p = strings.ToUpper(strings.TrimSpace(p))
		  if p == "" {
			   continue
		   }
		   if peer == "" || peer == p || p == ProtoLOCAL || peer == ProtoLOCAL || peer == ProtoUNHOSTED {
			   return p, true
		  }
	   }
	  return "", false
   }
