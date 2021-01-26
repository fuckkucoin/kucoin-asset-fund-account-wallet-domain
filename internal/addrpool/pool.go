package addrpool

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Binding struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Family string `json:"family"`
	Address  string    `json:"address"`
	BoundAt  time.Time `json:"bound_at"`
	Cooldown time.Time `json:"reuse_after"`
	Allocated  bool      `json:"allocated"`
	LastBeat   time.Time `json:"last_beat,omitempty"`
	LeaseUntil time.Time `json:"lease_until,omitempty"`
}

type DeriveFunc func(userID, chain string) string

type Pool struct {
	mu     sync.Mutex
	byKey  map[string]*Binding
	byAddr map[string]string
	derive DeriveFunc
	reuse  time.Duration
	clock  func() time.Time
	sql       *persist.SQLDB
	guardHook AllocateGuardHook // 防护钩子，可选；由 dustguard 注入
}

   func (p *Pool) SetDerive(fn DeriveFunc) {
	   if p != nil && fn != nil {
		  p.derive = fn
	  }
   }

   func New(derive DeriveFunc, reuse time.Duration) *Pool {
	   if derive == nil {
		  derive = wallet.DeriveAddress
	   }
	  if reuse <= 0 {
		   reuse = 24 * time.Hour
	  }
	  return &Pool{
		  byKey: map[string]*Binding{}, byAddr: map[string]string{},
		   derive: derive, reuse: reuse, clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func family(chain string) string {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  if wallet.IsEVM(chain) {
		   return "EVM"
	   }
	  return chain
   }

  func key(userID, chain string) string {
	  return strings.TrimSpace(userID) + "|" + family(chain)
   }

// Allocate 给用户在该链领充值地址。已绑定则复用；冷却未满禁止换地址。
// 入参：userID、chain。出参：Binding，地址必须过 ValidChainAddress。
// 派生结果已被别人占用则失败，不得串户。
   func (p *Pool) Allocate(userID, chain string) (Binding, error) {
	   if p == nil || p.derive == nil {
		  return Binding{}, fmt.Errorf("address pool unavailable")
	   }
	   userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  if userID == "" || chain == "" {
		   return Binding{}, fmt.Errorf("user_id and chain required")
	   }
	   if err := wallet.ValidChainAddress(chain, p.derive(userID, chain)); err != nil && p.derive(userID, chain) == "" {
		  return Binding{}, fmt.Errorf("derive failed for %s", chain)
	  }
	  addr := p.derive(userID, chain)
	  if addr == "" || wallet.ValidChainAddress(chain, addr) != nil {
		   return Binding{}, fmt.Errorf("deposit address unavailable for %s", chain)
	   }
	   // 防护钩子：若地址处于隔离或高攻击分值，拒绝分配（不影响派生逻辑）。
	  if p.guardHook != nil {
		   if err := p.guardHook.Check(ctxBackground(), chain, addr, userID); err != nil {
			  return Binding{}, err
		  }
	   }
	  k := key(userID, chain)
	   now := p.clock()
	  p.mu.Lock()
	   defer p.mu.Unlock()
	  if cur, ok := p.byKey[k]; ok && cur.Allocated {
		   cp := *cur
		  return cp, nil
	  }
	   if owner, ok := p.byAddr[strings.ToLower(addr)]; ok && owner != k {
		  return Binding{}, fmt.Errorf("address already bound")
	   }
	  b := &Binding{
		   UserID: userID, Chain: chain, Family: family(chain), Address: addr,
		  BoundAt: now, Cooldown: now.Add(p.reuse), Allocated: true,
	  }
	  p.byKey[k] = b
	   p.byAddr[strings.ToLower(addr)] = k
	   cp := *b
	   if err := p.persistBind(cp); err != nil {
		  delete(p.byKey, k)
		   delete(p.byAddr, strings.ToLower(addr))
		  return Binding{}, err
	   }
	   return cp, nil
  }

   func (p *Pool) Lookup(userID, chain string) (Binding, bool) {
	  if p == nil {
		   return Binding{}, false
	   }
	   p.mu.Lock()
	   b, ok := p.byKey[key(userID, chain)]
	  if ok && b.Allocated {
		   cp := *b
		   p.mu.Unlock()
		  return cp, true
	  }
	  p.mu.Unlock()
	   if loaded, found := p.loadSQL(userID, chain); found {
		  p.mu.Lock()
		   cp := loaded
		  p.byKey[key(userID, chain)] = &cp
		  p.byAddr[strings.ToLower(loaded.Address)] = key(userID, chain)
		   p.mu.Unlock()
		  return loaded, true
	  }
	  return Binding{}, false
   }

  func (p *Pool) ByAddress(addr string) (Binding, bool) {
	   if p == nil {
		  return Binding{}, false
	   }
	   p.mu.Lock()
	   k, ok := p.byAddr[strings.ToLower(strings.TrimSpace(addr))]
	  if ok {
		  b := p.byKey[k]
		   if b != nil {
			   cp := *b
			   p.mu.Unlock()
			  return cp, true
		   }
	   }
	   p.mu.Unlock()
	  if !p.sqlOn() {
		   return Binding{}, false
	   }
	  got, err := p.loadByAddr(addr)
	  if err != nil {
		  return Binding{}, false
	  }
	  return got, true
   }

// Release 冷却过了才能解绑。未过返回错误，避免同一地址立刻分给别人。

  func (p *Pool) Release(userID, chain string) error {
	  if p == nil {
		  return fmt.Errorf("address pool unavailable")
	   }
	   k := key(userID, chain)
	  now := p.clock()
	  p.mu.Lock()
	  defer p.mu.Unlock()
	   b, ok := p.byKey[k]
	  if !ok || !b.Allocated {
		  return nil
	  }
	  if now.Before(b.Cooldown) {
		  return fmt.Errorf("address reuse cooldown")
	  }
	   delete(p.byAddr, strings.ToLower(b.Address))
	  b.Allocated = false
	   if err := p.persistRelease(userID, chain); err != nil {
		  b.Allocated = true
		  p.byAddr[strings.ToLower(b.Address)] = k
		  return err
	  }
	  return nil
  }

  func (p *Pool) List(userID string) []Binding {
	   if p == nil {
		   return nil
	   }
	   if p.sqlOn() {
		  got, err := p.listSQL(userID)
		   if err != nil {
			  return nil
		   }
		   return got
	   }
	   p.mu.Lock()
	   defer p.mu.Unlock()
	   out := make([]Binding, 0)
	  for _, b := range p.byKey {
		  if b.UserID == userID && b.Allocated {
			   out = append(out, *b)
		   }
	   }
	   return out
  }
