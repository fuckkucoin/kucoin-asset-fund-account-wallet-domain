package destlimit

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ProposeOverride 登记待确认覆写。此时不得提高限额。

  func (e *Engine) ProposeOverride(userID, asset, clusterID, extra, actor, reason string, ttl time.Duration) (Override, error) {
	  if e == nil {
		   return Override{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return Override{}, err
	   }
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return Override{}, err
	  }
	   act, err := validate.Actor(actor)
	  if err != nil {
		   return Override{}, ErrDual
	   }
	   reason = strings.TrimSpace(reason)
	   if reason == "" || len(reason) > 200 {
		  return Override{}, ErrOverride
	   }
	   amt, err := money.Parse(extra)
	  if err != nil {
		  return Override{}, err
	  }
	  if ttl <= 0 || ttl > 7*24*time.Hour {
		  return Override{}, ErrOverride
	   }
	   clusterID = strings.TrimSpace(clusterID)
	  now := e.now()
	  row := Override{
		  ID:     uuid.NewString(),
		   UserID: uid, Asset: as, ClusterID: clusterID,
		   Extra: amt.String(), ActorA: act, Reason: reason, State: OvPending,
		   ExpiresAt: now.Add(ttl), CreatedAt: now,
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   e.overrides[row.ID] = &row
	   if err := e.persistOverride(row); err != nil {
		   delete(e.overrides, row.ID)
		  return Override{}, err
	   }
	   e.pushAudit(Audit{
		   Kind: AuditPropose, Subject: row.ID, Before: "", After: OvPending,
		  Actor: act, Detail: "extra=" + row.Extra, At: now,
	  })
	  return row, nil
   }

// ConfirmOverride 第二人确认。确认人必须与提案人不同，且记录仍在有效期内。
  func (e *Engine) ConfirmOverride(id, actor string) (Override, error) {
	  if e == nil {
		   return Override{}, ErrUnavailable
	  }
	   act, err := validate.Actor(actor)
	   if err != nil {
		  return Override{}, ErrDual
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   o := e.overrides[strings.TrimSpace(id)]
	  if o == nil {
		   return Override{}, ErrNotFound
	  }
	  now := e.now()
	   if !o.ExpiresAt.After(now) {
		  if _, err := e.killOverride(o, now, "confirm past expiry"); err != nil {
			   return Override{}, err
		   }
		  return Override{}, ErrExpired
	   }
	  if o.State != OvPending {
		  return Override{}, ErrOverride
	   }
	  if act == o.ActorA {
		   return Override{}, ErrDual
	  }
	   o.ActorB = act
	  o.State = OvActive
	  o.ConfirmedAt = now
	   if err := e.persistOverride(*o); err != nil {
		   o.ActorB = ""
		   o.State = OvPending
		  o.ConfirmedAt = time.Time{}
		   return Override{}, err
	   }
	  e.pushAudit(Audit{
		   Kind: AuditConfirm, Subject: o.ID, Before: OvPending, After: OvActive,
		  Actor: act, Detail: "expires=" + o.ExpiresAt.Format(time.RFC3339), At: now,
	   })
	  return *o, nil
  }

// GetOverride 读取覆写。若已过期则就地失效。
  func (e *Engine) GetOverride(id string) (Override, error) {
	  if e == nil {
		   return Override{}, ErrUnavailable
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  o := e.overrides[strings.TrimSpace(id)]
	  if o == nil {
		   return Override{}, ErrNotFound
	  }
	   now := e.now()
	  if o.State == OvActive && !o.ExpiresAt.After(now) {
		  got, err := e.killOverride(o, now, "read past expiry")
		  return got, err
	  }
	  return *o, nil
  }

  func (e *Engine) ListOverrides(userID, asset string) []Override {
	   if e == nil {
		   return nil
	  }
	  userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  now := e.now()
	   out := make([]Override, 0)
	  for _, o := range e.overrides {
		   if o.State == OvActive && !o.ExpiresAt.After(now) {
			  _, _ = e.killOverride(o, now, "list past expiry")
		  }
		   if userID != "" && o.UserID != userID {
			  continue
		   }
		   if asset != "" && o.Asset != asset {
			  continue
		  }
		  out = append(out, *o)
	  }
	  return out
   }

// ExpireOverrides 扫描并失效过期覆写。

   func (e *Engine) ExpireOverrides(now time.Time) (int, error) {
	   if e == nil {
		  return 0, ErrUnavailable
	  }
	  if now.IsZero() {
		   now = e.now()
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   n := 0
	  for _, o := range e.overrides {
		   if o.State != OvActive && o.State != OvPending {
			  continue
		   }
		   if o.ExpiresAt.After(now) {
			   continue
		  }
		  if _, err := e.killOverride(o, now, "worker"); err != nil {
			  return n, err
		   }
		   n++
	   }
	   return n, nil
   }

   func (e *Engine) killOverride(o *Override, now time.Time, detail string) (Override, error) {
	   if o.State == OvExpired || o.State == OvRevoked {
		  return *o, ErrExpired
	  }
	  prev := o.State
	   o.State = OvExpired
	  if err := e.persistOverride(*o); err != nil {
		  o.State = prev
		   return Override{}, err
	   }
	  e.pushAudit(Audit{
		  Kind: AuditOvExpire, Subject: o.ID, Before: prev, After: OvExpired,
		   Detail: detail, At: now,
	   })
	   return *o, nil
  }

// activeExtra 返回仍有效的覆写增量。簇号为空的覆写对该用户该资产全部簇生效。
// 调用方须已持有 e.mu。

   func (e *Engine) activeExtra(user, asset, cluster string, now time.Time) (string, *Override) {
	   var best *Override
	   sum := decimal.Zero
	   for _, o := range e.overrides {
		   if o.UserID != user || o.Asset != asset {
			  continue
		   }
		  if o.State == OvActive && !o.ExpiresAt.After(now) {
			  _, _ = e.killOverride(o, now, "eval past expiry")
			  continue
		  }
		  if o.State != OvActive {
			   continue
		   }
		  if o.ClusterID != "" && o.ClusterID != cluster {
			  continue
		  }
		   ex, err := decimal.NewFromString(o.Extra)
		  if err != nil {
			  continue
		   }
		   sum = sum.Add(ex)
		   best = o
	   }
	   if best == nil {
		  return "0", nil
	  }
	  return sum.String(), best
   }
