// @FileName: abuse.go
// @Summary: 入金刷充熔断。速度键只有 dep: 前缀，形如 dep:{用户}|{链}|{资产}。
// 窗口内笔数达到上限后，拒绝新的观察写和确认写，直到窗口过去。
// 提现申请不进入本闸，读面里提现请求始终不算被这道熔断关闭。
// @Author: KnoxRay
// @Created: 2020.10.11

package deposit

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/limitgate"
	"account-assets.kucoin.net/funding/internal/usage"
)

// AbuseCircuit 入金写速度。超限后窗口内拒绝新的观察与确认写。

type AbuseCircuit struct {
	mu sync.Mutex
	use *usage.Store
	cap   int
	win   time.Duration
	open  map[string]time.Time
	clock func() time.Time
	opened int
}

   func NewCircuit() *AbuseCircuit {
	   return &AbuseCircuit{
		   use:   usage.New(),
		   cap:   20,
		  win:   time.Minute,
		   open:  map[string]time.Time{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

  func (c *AbuseCircuit) now() time.Time {
	   if c == nil || c.clock == nil {
		   return time.Now().UTC()
	   }
	  return c.clock()
  }

// SetCap 调整窗口笔数。cap 或窗口小于等于零会关掉熔断，直接拒绝。

   func (c *AbuseCircuit) SetCap(cap int, win time.Duration) error {
	  if c == nil {
		   return ErrAbuseKey
	   }
	   if cap <= 0 || win <= 0 {
		   return ErrAbuseKey
	  }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	  c.cap = cap
	  c.win = win
	   return nil
   }

// BindUsage 换外部占用表。nil 则保留内部表。只通过 AddDep 写入。
   func (c *AbuseCircuit) BindUsage(use *usage.Store) {
	   if c == nil || use == nil {
		   return
	  }
	  c.mu.Lock()
	   c.use = use
	   c.mu.Unlock()
   }

// Admit 记一笔入金写。键必须是 dep: 前缀，拼不出来就失败，不会落到提现占用上。
// 熔断还开着时直接拒绝，不再把这笔算进速度，避免拒绝本身把窗口越撑越长。

  func (c *AbuseCircuit) Admit(user, chain, asset string) error {
	   if c == nil {
		  return nil
	   }
	   key, err := usage.DepVelocityKey(user, chain, asset)
	   if err != nil {
		   return ErrAbuseKey
	  }
	   if !usage.IsDepKey(key) {
		  return ErrAbuseKey
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	   if until, ok := c.open[key]; ok && c.now().Before(until) {
		   return ErrDepositSpam
	  }
	   out, err := limitgate.CheckDeposit(c.use, user, chain, asset, c.cap, c.win)
	  if err != nil {
		  return err
	   }
	   if !out.Allow {
		  c.open[key] = c.now().Add(c.win)
		   c.opened++
		  return ErrDepositSpam
	  }
	   if err := c.use.AddDep(key, "1"); err != nil {
		  return err
	  }
	  return nil
   }

// Open 该 dep: 键当前是否熔断。
   func (c *AbuseCircuit) Open(user, chain, asset string) bool {
	  if c == nil {
		  return false
	   }
	  key, err := usage.DepVelocityKey(user, chain, asset)
	   if err != nil || !usage.IsDepKey(key) {
		  return false
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	  until, ok := c.open[key]
	  return ok && c.now().Before(until)
  }

// WithdrawRequest 提现请求不因本熔断关闭。
  func (c *AbuseCircuit) WithdrawRequest() error {
	   return limitgate.WithdrawUntouched()
   }

// Snapshot 读面：入金写是否被挡，提现请求是否被挡。

type AbuseSnap struct {
	Cap int `json:"cap"`
	WindowSec int `json:"window_sec"`
	OpenCircuits   int  `json:"open_circuits"`
	DepositBlocked bool `json:"deposit_writes_blocked"`
	WithdrawBlocked bool `json:"withdraw_request_blocked"`
	KeyPrefix string `json:"key_prefix"`
}

  func (c *AbuseCircuit) Snapshot(user, chain, asset string) AbuseSnap {
	   snap := AbuseSnap{KeyPrefix: usage.DepPrefix, WithdrawBlocked: false}
	   if c == nil {
		   return snap
	  }
	  c.mu.Lock()
	   snap.Cap = c.cap
	   snap.WindowSec = int(c.win / time.Second)
	   n := 0
	   now := c.now()
	   for _, until := range c.open {
		  if now.Before(until) {
			  n++
		   }
	   }
	  snap.OpenCircuits = n
	   c.mu.Unlock()
	  snap.DepositBlocked = c.Open(user, chain, asset)
	   _ = c.WithdrawRequest()
	   return snap
   }
