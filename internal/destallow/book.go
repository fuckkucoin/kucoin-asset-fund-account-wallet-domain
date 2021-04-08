package destallow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type Entry struct {
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Label   string    `json:"label,omitempty"`
	AddedAt time.Time `json:"added_at"`
}

type Book struct {
	mu   sync.Mutex
	rows map[string]Entry
	clock func() time.Time
	sql   *persist.SQLDB
	kv    persist.KV
	cooldown time.Duration
}

   func New() *Book {
	   return &Book{rows: map[string]Entry{}, clock: func() time.Time { return time.Now().UTC() }}
   }

   func key(userID, chain, addr string) string {
	   return exactKey(userID, chain, addr)
   }

   func (b *Book) Add(userID, chain, addr, label string) (Entry, error) {
	  userID = strings.TrimSpace(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  addr = strings.TrimSpace(addr)
	  if userID == "" || chain == "" || addr == "" {
		  return Entry{}, fmt.Errorf("user, chain and address required")
	   }
	   b.mu.Lock()
	   now := b.clock()
	  e := Entry{UserID: userID, Chain: chain, Address: addr, Label: label, AddedAt: now}
	  if prev, ok := b.rows[key(userID, chain, addr)]; ok && prev.Address == addr && !prev.AddedAt.IsZero() {
		   e.AddedAt = prev.AddedAt
	  }
	  b.rows[key(userID, chain, addr)] = e
	  b.mu.Unlock()
	  if err := b.persistAdd(e); err != nil {
		   b.mu.Lock()
		   delete(b.rows, key(userID, chain, addr))
		   b.mu.Unlock()
		   return Entry{}, err
	   }
	   return e, nil
  }

  func (b *Book) Remove(userID, chain, addr string) {
	   if b == nil {
		   return
	   }
	  if b.sqlOn() {
		  if err := sqlrepo.NewDestAllow(b.sql).Delete(context.Background(), userID, chain, addr); err != nil {
			  return
		   }
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   delete(b.rows, key(userID, chain, addr))
  }

   func (b *Book) Allowed(userID, chain, addr string) bool {
	   return b.Require(userID, chain, addr) == nil
   }

   func (b *Book) lookup(userID, chain, addr string) (Entry, bool, error) {
	  if b == nil {
		  return Entry{}, false, nil
	   }
	   if b.sqlOn() {
		   e, ok, err := sqlrepo.NewDestAllow(b.sql).Get(context.Background(), userID, chain, addr)
		   if err != nil {
			   return Entry{}, false, fmt.Errorf("%w: %v", ErrStore, err)
		   }
		  if !ok {
			  return Entry{}, false, nil
		  }
		  return Entry{UserID: e.UserID, Chain: e.Chain, Address: e.Address, Label: e.Label, AddedAt: e.AddedAt}, true, nil
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  e, ok := b.rows[key(userID, chain, addr)]
	   return e, ok, nil
  }

   func (b *Book) coolOK(added time.Time) bool {
	  if b == nil || b.cooldown <= 0 || added.IsZero() {
		   return true
	  }
	  now := time.Now().UTC()
	   if b.clock != nil {
		  now = b.clock()
	   }
	  return !added.Add(b.cooldown).After(now)
  }

  func (b *Book) SetCooldown(d time.Duration) *Book {
	   if b != nil {
		   b.cooldown = d
	   }
	   return b
  }

// Require 未登记则失败即关。新地址冷却未满同样拒绝。存储故障不得伪装成未登记。调用方不得在未过白名单时冻账。
   func (b *Book) Require(userID, chain, addr string) error {
	  if b.BreakGlassLive(userID, chain, addr) {
		   return nil
	  }
	   if err := b.CheckGeoChain(chain, ""); err != nil {
		   metrics.FundGates.WithLabelValues("destallow", "policy").Inc()
		  b.NoteBreach(userID, chain, addr, "policy")
		   return err
	   }
	  e, ok := b.GetExact(userID, chain, addr)
	   if !ok {
		   if b.sqlOn() {
			  if _, _, err := b.lookup(userID, chain, addr); err != nil {
				   metrics.FundGates.WithLabelValues("destallow", "store").Inc()
				  return err
			  }
		  }
		  metrics.FundGates.WithLabelValues("destallow", "missing").Inc()
		   b.NoteBreach(userID, chain, addr, "missing")
		   return ErrMissing
	   }
	  if !b.coolOK(e.AddedAt) {
		   metrics.FundGates.WithLabelValues("destallow", "cooldown").Inc()
		   b.NoteBreach(userID, chain, addr, "cooldown")
		   return ErrCooldown
	  }
	  if err := b.CheckLive(userID, chain, addr); err != nil {
		   if errors.Is(err, ErrExpired) {
			  metrics.FundGates.WithLabelValues("destallow", "expired").Inc()
		  }
		   return err
	  }
	   return nil
  }

  func (b *Book) List(userID string) []Entry {
	   if b == nil {
		   return nil
	   }
	   if b.sqlOn() {
		   got, err := sqlrepo.NewDestAllow(b.sql).ListUser(context.Background(), userID)
		   if err != nil {
			  return nil
		  }
		  out := make([]Entry, 0, len(got))
		   for _, e := range got {
			   out = append(out, Entry{UserID: e.UserID, Chain: e.Chain, Address: e.Address, Label: e.Label, AddedAt: e.AddedAt})
		   }
		   return out
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  out := make([]Entry, 0)
	  for _, e := range b.rows {
		  if e.UserID == userID {
			  out = append(out, e)
		   }
	  }
	   return out
   }

   func (b *Book) Len() int {
	   if b == nil {
		   return 0
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   return len(b.rows)
   }
