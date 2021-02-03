package addrpool

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

const defaultLeaseTTL = 30 * time.Minute

// Heartbeat 续租已分配一对一地址。未分配失败关闭。

  func (p *Pool) Heartbeat(userID, chain string) (Binding, error) {
	   if p == nil {
		  return Binding{}, ErrAssignOwner
	  }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		   return Binding{}, err
	  }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		   return Binding{}, err
	   }
	   if wallet.RequiresFamMemo(ch) {
		  return Binding{}, ErrSharedRelease
	  }
	   now := p.clock()
	   ttl := p.reuse
	   if ttl <= 0 {
		  ttl = defaultLeaseTTL
	   }
	  p.mu.Lock()
	  defer p.mu.Unlock()
	   k := key(uid, ch)
	   b, ok := p.byKey[k]
	   if !ok || b == nil || !b.Allocated {
		  return Binding{},
			   ErrAssignOwner
	  }
	   b.LastBeat = now
	  b.LeaseUntil = now.Add(ttl)
	   cp := *b
	  if err := p.persistBind(cp); err != nil {
		  return Binding{}, err
	   }
	  return cp, nil
   }

// ExpireReclaimTick 回收租约过期的一对一绑。未设租约的条跳过。
  func (p *Pool) ExpireReclaimTick() int {
	   if p == nil {
		   return 0
	  }
	   now := p.clock()
	   p.mu.Lock()
	   defer p.mu.Unlock()
	   n := 0
	   for _, b := range p.byKey {
		  if b == nil || !b.Allocated || b.LeaseUntil.IsZero() || !now.After(b.LeaseUntil) {
			  continue
		   }
		  prev := *b
		  delete(p.byAddr, strings.ToLower(b.Address))
		   b.Allocated = false
		  if err := p.persistRelease(b.UserID, b.Chain); err != nil {
			  *b = prev
			   p.byAddr[strings.ToLower(b.Address)] = key(b.UserID, b.Chain)
			  continue
		  }
		   n++
	   }
	   return n
   }

// ExpireReservedTick 回收库存中过期 reserved。

  func (inv *Inventory) ExpireReservedTick() int {
	   if inv == nil {
		  return 0
	  }
	  now := inv.clock()
	   ttl := inv.reuse
	   if ttl <= 0 {
		  ttl = defaultLeaseTTL
	   }
	  inv.mu.Lock()
	   defer inv.mu.Unlock()
	  n := 0
	  for _, s := range inv.slots {
		  if s.Status != SlotReserved {
			  continue
		  }
		   if s.Reserved.IsZero() || !now.After(s.Reserved.Add(ttl)) {
			   continue
		  }
		   prev := *s
		   s.Status = SlotFree
		   s.UserID = ""
		   s.Reserved = time.Time{}
		  if err := inv.persistSlot(s); err != nil {
			   *s = prev
			   continue
		  }
		   n++
	  }
	   return n
   }

type expireHost struct {
	pool *Pool
	inv  *Inventory
}

   func (h expireHost) tick(ctx context.Context) error {
	  _ = ctx
	  if h.pool != nil {
		  _ = h.pool.ExpireReclaimTick()
	   }
	   if h.inv != nil {
		  _ = h.inv.ExpireReservedTick()
	   }
	  return nil
   }

// RunExpire 用 workerloop 周期回收过期租约。

  func RunExpire(ctx context.Context, pool *Pool, inv *Inventory, log zerolog.Logger) error {
	  h := expireHost{pool: pool, inv: inv}
	  cfg := workerloop.Config{
		   Name: "addrpool-expire", Interval: 30 * time.Second,
		  MaxBackoff: 30 * time.Second, FailThresh: 8, CircuitPause: 15 * time.Second,
	   }
	  return workerloop.Run(ctx, cfg, log, h.tick)
   }

// StartExpire 后台节拍。 Register 挂上。
   func StartExpire(pool *Pool, inv *Inventory, log zerolog.Logger) {
	  if pool == nil && inv == nil {
		  return
	   }
	  go func() {
		  _ = RunExpire(context.Background(), pool, inv, log)
	  }()
  }
