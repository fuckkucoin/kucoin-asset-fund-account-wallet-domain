package destallow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

const kindBreakGlass = "destallow.breakglass"

var ErrGlass = errors.New("destallow break-glass invalid")

   func init() {
	   apierr.Register(ErrGlass, http.StatusBadRequest)
  }

// BreakGlass 紧急旁路记录。ApproverA/B 只存标识。
type BreakGlass struct {
	UserID string `json:"user_id"`
	Chain     string `json:"chain"`
	Address   string `json:"address"`
	ApproverA string `json:"approver_a"`
	ApproverB string `json:"approver_b"`
	Reason string `json:"reason"`
	ExpiresAt time.Time `json:"expires_at"`
	Actor     string    `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

   func glassKey(userID, chain, addr string) string {
	  return exactKey(userID, chain, addr)
   }

// PutBreakGlass 落紧急旁路。双审批字段与 TTL 必填；不核验身份令牌。
   func (b *Book) PutBreakGlass(ctx context.Context, g BreakGlass, ttl time.Duration) (BreakGlass, error) {
	   if b == nil {
		  return BreakGlass{}, ErrMissing
	   }
	  if b.kv == nil {
		  return BreakGlass{}, ErrStore
	  }
	   uid, ch, addr, err := normUserChainAddr(g.UserID, g.Chain, g.Address)
	  if err != nil {
		  return BreakGlass{}, err
	  }
	  a := strings.TrimSpace(g.ApproverA)
	  bb := strings.TrimSpace(g.ApproverB)
	   if a == "" || bb == "" || a == bb {
		  return BreakGlass{}, ErrGlass
	   }
	  if _, err := validate.Actor(a); err != nil {
		   return BreakGlass{}, ErrGlass
	  }
	  if _, err := validate.Actor(bb); err != nil {
		   return BreakGlass{}, ErrGlass
	   }
	  if ttl <= 0 {
		  return BreakGlass{}, fmt.Errorf("destallow break-glass ttl required")
	   }
	  reason := strings.TrimSpace(g.Reason)
	  if reason == "" || len(reason) > 128 {
		  return BreakGlass{}, ErrGlass
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   now := b.now()
	   out := BreakGlass{
		  UserID: uid, Chain: ch, Address: addr,
		   ApproverA: a,
		   ApproverB: bb, Reason: reason,
		  ExpiresAt: now.Add(ttl), Actor: strings.TrimSpace(g.Actor), CreatedAt: now,
	  }
	  body, err := json.Marshal(out)
	  if err != nil {
		  return BreakGlass{}, err
	   }
	  key := glassKey(uid, ch, addr)
	  _, existed, err := persist.GetOrInit(ctx, b.kv, kindBreakGlass, key, body)
	  if err != nil {
		  return BreakGlass{}, err
	   }
	  if existed {
		   if err := persist.Replace(ctx, b.kv, kindBreakGlass, key, body); err != nil {
			  return BreakGlass{}, err
		  }
	   }
	   b.noteAudit(out.Actor, "break_glass", uid, ch, addr, "", a+"+"+bb)
	   return out, nil
  }

   func (b *Book) loadGlass(ctx context.Context, userID, chain, addr string) (BreakGlass, bool) {
	  if b == nil || b.kv == nil {
		   return BreakGlass{}, false
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   row, err := b.kv.Get(ctx, kindBreakGlass, glassKey(userID, chain, addr))
	  if err != nil {
		  return BreakGlass{}, false
	  }
	  var g BreakGlass
	  if json.Unmarshal(row.Body, &g) != nil {
		  return BreakGlass{}, false
	  }
	   return g, true
   }

// BreakGlassLive 精确目标是否有未过期旁路。过期摘除，不得放行。
   func (b *Book) BreakGlassLive(userID, chain, addr string) bool {
	  g,
		  ok := b.loadGlass(context.Background(), userID, chain, addr)
	  if !ok {
		   return false
	   }
	   if g.ExpiresAt.IsZero() || !g.ExpiresAt.After(b.now()) {
		   b.dropGlass(context.Background(), userID, chain, addr)
		  return false
	   }
	  if strings.TrimSpace(g.ApproverA) == "" || strings.TrimSpace(g.ApproverB) == "" {
		   return false
	   }
	   return true
  }

   func (b *Book) dropGlass(ctx context.Context, userID, chain, addr string) {
	  if b == nil || b.kv == nil {
		  return
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	  key := glassKey(userID, chain, addr)
	  row, err := b.kv.Get(ctx, kindBreakGlass, key)
	  if err != nil {
		  return
	   }
	  _ = b.kv.Delete(ctx,
		   kindBreakGlass, key, row.Fence)
  }

// GetBreakGlass 读旁路记录。过期当未登记。
  func (b *Book) GetBreakGlass(userID, chain, addr string) (BreakGlass, error) {
	  if b == nil {
		   return BreakGlass{}, ErrMissing
	  }
	  uid, ch, a, err := normUserChainAddr(userID, chain, addr)
	   if err != nil {
		   return BreakGlass{}, err
	  }
	  g, ok := b.loadGlass(context.Background(), uid, ch, a)
	   if !ok || !b.BreakGlassLive(uid, ch, a) {
		   return BreakGlass{}, ErrMissing
	   }
	  return g, nil
  }
