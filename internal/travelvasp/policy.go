package travelvasp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindTravelPolicy = "travel.corridor.policy"

// CorridorPolicy 单走廊策略。MinAmount 可空，空则回落门槛表。
type CorridorPolicy struct {
	Corridor string `json:"corridor"`
	Asset string `json:"asset,omitempty"`
	MinAmount string `json:"min_amount,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	Protocols  []string `json:"protocols,omitempty"`
	FailClosed bool     `json:"fail_closed"`
}

// PolicyBook 走廊策略热表。

type PolicyBook struct {
	mu   sync.RWMutex
	rows map[string]CorridorPolicy
	kv persist.KV
}

   func NewPolicyBook() *PolicyBook {
	   return &PolicyBook{rows: map[string]CorridorPolicy{}}
   }

  func (b *PolicyBook) BindKV(kv persist.KV) *PolicyBook {
	   if b != nil && kv != nil {
		  b.kv = kv
	  }
	  return b
   }

  func policyKey(corridor, asset string) string {
	  c := strings.ToUpper(strings.TrimSpace(corridor))
	  a := strings.ToUpper(strings.TrimSpace(asset))
	   if c == "" {
		  c = "DEFAULT"
	  }
	   return c + "|" + a
  }

// Put 写入一条策略并落 KV。

  func (b *PolicyBook) Put(ctx context.Context, row CorridorPolicy) error {
	  if b == nil {
		   return ErrUnavailable
	   }
	   row.Corridor = strings.ToUpper(strings.TrimSpace(row.Corridor))
	  row.Asset = strings.ToUpper(strings.TrimSpace(row.Asset))
	   row.Protocol = strings.ToUpper(strings.TrimSpace(row.Protocol))
	  row.MinAmount = strings.TrimSpace(row.MinAmount)
	  if row.Corridor == "" {
		  row.Corridor = "DEFAULT"
	   }
	   prefs := make([]string, 0, len(row.Protocols))
	   for _, p := range row.Protocols {
		  p = strings.ToUpper(strings.TrimSpace(p))
		   if p == "" {
			   continue
		  }
		  if _, err := normalizeProtocol(p, p == ProtoUNHOSTED); err != nil {
			  return err
		  }
		  prefs = append(prefs, p)
	  }
	   row.Protocols = prefs
	   if row.Protocol != "" {
		   if _, err := normalizeProtocol(row.Protocol, row.Protocol == ProtoUNHOSTED); err != nil {
			   return err
		   }
	   } else if len(prefs) > 0 {
		   row.Protocol = prefs[0]
	  }
	  b.mu.Lock()
	   if b.rows == nil {
		  b.rows = map[string]CorridorPolicy{}
	   }
	  b.rows[policyKey(row.Corridor, row.Asset)] = row
	   b.mu.Unlock()
	   return b.persist(ctx, row)
  }

  func (b *PolicyBook) persist(ctx context.Context, row CorridorPolicy) error {
	   if b == nil || b.kv == nil {
		  return nil
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   body, err := json.Marshal(row)
	  if err != nil {
		   return err
	  }
	  key := policyKey(row.Corridor, row.Asset)
	  _, existed, err := persist.GetOrInit(ctx, b.kv, kindTravelPolicy, key, body)
	  if err != nil {
		  return err
	   }
	  if existed {
		  return persist.Replace(ctx, b.kv, kindTravelPolicy, key, body)
	  }
	  return nil
   }

// Reload 从 KV 热加载策略。无行保留内存。
  func (b *PolicyBook) Reload(ctx context.Context) error {
	   if b == nil || b.kv == nil {
		  return nil
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   rows, err := b.kv.List(ctx, kindTravelPolicy, 400)
	  if err != nil {
		  return err
	   }
	   next := map[string]CorridorPolicy{}
	   for _, row := range rows {
		  var p CorridorPolicy
		  if json.Unmarshal(row.Body, &p) != nil {
			  continue
		   }
		  p.Corridor = strings.ToUpper(strings.TrimSpace(p.Corridor))
		  p.Asset = strings.ToUpper(strings.TrimSpace(p.Asset))
		  if p.Corridor == "" {
			  p.Corridor = "DEFAULT"
		  }
		  next[policyKey(p.Corridor, p.Asset)] = p
	   }
	  b.mu.Lock()
	  b.rows = next
	   b.mu.Unlock()
	   return nil
   }

// Lookup 解析走廊策略。未命中返回零值 found=false。

   func (b *PolicyBook) Lookup(corridor, asset string) (CorridorPolicy, bool) {
	  if b == nil {
		  return CorridorPolicy{}, false
	   }
	   b.mu.RLock()
	   defer b.mu.RUnlock()
	  for _, k := range []string{
		   policyKey(corridor, asset),
		  policyKey(corridor, ""),
		  policyKey("DEFAULT", asset),
		  policyKey("DEFAULT", ""),
	  } {
		   if r, ok := b.rows[k]; ok {
			  return r, true
		   }
	   }
	   return CorridorPolicy{}, false
  }

// List 导出策略表。
  func (b *PolicyBook) List() []CorridorPolicy {
	  if b == nil {
		  return nil
	   }
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	  out := make([]CorridorPolicy,
		  0, len(b.rows))
	   for _, r := range b.rows {
		  out = append(out, r)
	   }
	   return out
   }

   func (s *Service) BindPolicy(book *PolicyBook) *Service {
	   if s != nil && book != nil {
		   s.policy = book
	   }
	   return s
  }

   func (s *Service) Policies() *PolicyBook {
	  if s == nil {
		  return nil
	   }
	   return s.policy
  }
