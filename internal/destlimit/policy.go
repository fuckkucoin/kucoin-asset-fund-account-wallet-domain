package destlimit

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

  func (e *Engine) PutPolicy(p Policy) (Policy, error) {
	  if e == nil {
		  return Policy{}, ErrUnavailable
	   }
	  norm, err := normalizePolicy(p, e.now())
	   if err != nil {
		  return Policy{}, err
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  maxV := int64(0)
	   for _, cur := range e.policies {
		   if cur != nil && cur.Asset == norm.Asset && cur.Version > maxV {
			  maxV = cur.Version
		  }
	  }
	  norm.Version = maxV + 1
	   cp := norm
	   e.policies[norm.ID] = &cp
	   e.generation++
	  if err := e.persistPolicy(cp); err != nil {
		  delete(e.policies, norm.ID)
		   return Policy{}, err
	  }
	   e.pushAudit(Audit{
		  Kind: AuditPolicy, Subject: cp.ID, Before: "", After: cp.ClusterMax,
		  Detail: "asset=" + cp.Asset + " version=" + itoa64(cp.Version), At: cp.UpdatedAt,
	   })
	  return cp, nil
  }

   func (e *Engine) GetPolicy(id string) (Policy, error) {
	   if e == nil {
		  return Policy{}, ErrUnavailable
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   p := e.policies[strings.TrimSpace(id)]
	  if p == nil {
		  return Policy{}, ErrNotFound
	  }
	  return *p, nil
  }

  func (e *Engine) ListPolicies() []Policy {
	   if e == nil {
		   return nil
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  out := make([]Policy, 0, len(e.policies))
	  for _, p := range e.policies {
		  out = append(out, *p)
	  }
	   return out
   }

// DisablePolicy 停用策略。停用后该资产若无其它启用策略，求值失败关闭。

  func (e *Engine) DisablePolicy(id, actor string) (Policy, error) {
	   if e == nil {
		  return Policy{}, ErrUnavailable
	  }
	   id = strings.TrimSpace(id)
	  actor = strings.TrimSpace(actor)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  p := e.policies[id]
	  if p == nil {
		   return Policy{}, ErrNotFound
	   }
	  before := "enabled"
	   p.Enabled = false
	   p.Version++
	  p.UpdatedAt = e.now()
	   if err := e.persistPolicy(*p); err != nil {
		  p.Enabled = true
		  p.Version--
		  return Policy{}, err
	   }
	  e.generation++
	  e.pushAudit(Audit{
		   Kind: AuditPolicy, Subject: id, Before: before, After: "disabled",
		  Actor: actor, At: p.UpdatedAt,
	  })
	  return *p, nil
   }

// Reload 热加载。有 SQL 时以库为准覆盖内存；无 SQL 时只提升代际。
   func (e *Engine) Reload() (int64, error) {
	  if e == nil {
		   return 0, ErrUnavailable
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   before := e.generation
	   if e.sqlOn() {
		   if err := e.loadPolicies(); err != nil {
			  return e.generation, err
		   }
	   }
	   e.generation++
	   e.pushAudit(Audit{
		  Kind: AuditReload, Subject: "policy", Before: itoa64(before), After: itoa64(e.generation),
		  Detail: "hot reload", At: e.now(),
	  })
	  return e.generation, nil
  }

  func (e *Engine) Generation() int64 {
	   if e == nil {
		   return 0
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  return e.generation
  }

   func newerPolicy(n, old *Policy) bool {
	   if n == nil || !n.Enabled {
		  return false
	  }
	  if old == nil {
		   return true
	   }
	   if n.Version != old.Version {
		   return n.Version > old.Version
	   }
	   return n.UpdatedAt.After(old.UpdatedAt)
   }

  func (e *Engine) pickPolicy(asset string) (Policy, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   var best *Policy
	  var star *Policy
	  for _, p := range e.policies {
		   if p == nil || !p.Enabled {
			  continue
		  }
		   if p.Asset == asset && newerPolicy(p, best) {
			   best = p
		  }
		   if p.Asset == "*" && newerPolicy(p, star) {
			   star = p
		   }
	  }
	   if best != nil {
		  return *best, nil
	  }
	  if star != nil {
		  return *star, nil
	   }
	  return Policy{}, &DenyError{Code: CodeNoPolicy, Reason: "no enabled limit policy"}
  }

   func normalizePolicy(p Policy, now time.Time) (Policy, error) {
	  p.ID = strings.TrimSpace(p.ID)
	  if p.ID == "" {
		  p.ID = "pol-" + uuid.NewString()
	   }
	   if len(p.ID) > 64 {
		   return Policy{}, ErrPolicy
	  }
	  p.Name = strings.TrimSpace(p.Name)
	   if p.Name == "" {
		   p.Name = p.ID
	  }
	  if len(p.Name) > 64 {
		   return Policy{}, ErrPolicy
	  }
	  asset := strings.TrimSpace(p.Asset)
	   if asset == "*" {
		   p.Asset = "*"
	  } else {
		   a, err := validate.Asset(asset)
		  if err != nil {
			  return Policy{}, err
		  }
		   p.Asset = a
	  }
	   var err error
	  p.ClusterMax, err = positiveAmt(p.ClusterMax)
	   if err != nil {
		  return Policy{}, err
	   }
	   p.UserMax, err = positiveAmt(p.UserMax)
	   if err != nil {
		   return Policy{}, err
	  }
	  p.DestMax, err = positiveAmt(p.DestMax)
	   if err != nil {
		   return Policy{}, err
	  }
	   if p.LowMul == "" {
		   p.LowMul = "1"
	  }
	   if p.MidMul == "" {
		   p.MidMul = "0.5"
	   }
	  if p.HighMul == "" {
		   p.HighMul = "0.25"
	   }
	   if p.SevereMul == "" {
		  p.SevereMul = "0.1"
	   }
	   for _, m := range []string{p.LowMul, p.MidMul, p.HighMul, p.SevereMul} {
		   d,
			   derr := decimal.NewFromString(strings.TrimSpace(m))
		   if derr != nil || !d.IsPositive() || d.GreaterThan(decimal.NewFromInt(10)) {
			  return Policy{}, ErrPolicy
		   }
	   }
	   p.LowMul = strings.TrimSpace(p.LowMul)
	  p.MidMul = strings.TrimSpace(p.MidMul)
	   p.HighMul = strings.TrimSpace(p.HighMul)
	  p.SevereMul = strings.TrimSpace(p.SevereMul)
	  if p.WindowSec <= 0 {
		   p.WindowSec = 86400
	  }
	  if p.WindowSec > 30*86400 {
		  return Policy{}, ErrPolicy
	  }
	   p.Enabled = true
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   p.UpdatedAt = now
	   return p, nil
  }

   func positiveAmt(s string) (string, error) {
	   a, err := money.Parse(s)
	   if err != nil {
		   return "", err
	  }
	   return a.String(), nil
   }

   func itoa64(n int64) string {
	  if n == 0 {
		  return "0"
	  }
	  neg := n < 0
	   if neg {
		  n = -n
	  }
	   var b [24]byte
	  i := len(b)
	   for n > 0 {
		   i--
		   b[i] = byte('0' + n%10)
		   n /= 10
	  }
	   if neg {
		  i--
		  b[i] = '-'
	  }
	  return string(b[i:])
   }
