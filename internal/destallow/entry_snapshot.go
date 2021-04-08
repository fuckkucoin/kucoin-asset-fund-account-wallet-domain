package destallow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// EntryMode 条目生命周期模式，供条目租约钉扎。不等于 ListGen 簿世代。
type EntryMode string

const (
	EntryModeNone       EntryMode = ""
	EntryModeStrict     EntryMode = "strict"
	EntryModeTemp       EntryMode = "temp"
	EntryModeBreakglass EntryMode = "breakglass"
	EntryModeCooldownGap EntryMode = "cooldown_gap"
)

// EntrySnapshot 单条目只读观察面。
type EntrySnapshot struct {
	EntryID string `json:"entry_id"`
	UserID  string `json:"user_id"`
	Chain string `json:"chain"`
	Address string `json:"address"`
	Label string `json:"label,omitempty"`
	Mode EntryMode `json:"mode"`
	InBook       bool      `json:"in_book"`
	Temp         bool      `json:"temp"`
	TempExpires  time.Time `json:"temp_expires_at,omitempty"`
	GlassLive    bool      `json:"breakglass_live"`
	GlassExpires time.Time `json:"breakglass_expires_at,omitempty"`
	CooldownFull bool      `json:"cooldown_full"`
	AddedAt time.Time `json:"added_at,omitempty"`
	Live    bool      `json:"live"`
	Reason string `json:"reason,omitempty"`
}

// ComputeEntryID 由 user/chain/addr 派生稳定条目标识（非 ListGen）。
   func ComputeEntryID(userID, chain, addr string) string {
	   userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   addr = strings.TrimSpace(addr)
	   sum := sha256.Sum256([]byte(userID + "|" + chain + "|" + addr))
	  return hex.EncodeToString(sum[:])
   }

// SnapshotEntry 观察一条目标的生命周期面。不签发租约。
   func (b *Book) SnapshotEntry(ctx context.Context, userID, chain, addr string) EntrySnapshot {
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   snap := EntrySnapshot{
		  EntryID: ComputeEntryID(userID, chain, addr),
		   UserID:  strings.TrimSpace(userID),
		  Chain:   strings.ToUpper(strings.TrimSpace(chain)),
		   Address: strings.TrimSpace(addr),
	  }
	  if b == nil {
		  snap.Reason = "book_nil"
		  return snap
	   }
	  if b.BreakGlassLive(userID, chain, addr) {
		   snap.GlassLive = true
		  snap.Live = true
		  snap.Mode = EntryModeBreakglass
		  if g, ok := b.loadGlass(ctx, snap.UserID, snap.Chain, snap.Address); ok {
			  snap.GlassExpires = g.ExpiresAt
		  }
	   }
	  e, ok := b.GetExact(userID, chain, addr)
	  if ok {
		   snap.InBook = true
		  snap.Label = e.Label
		   snap.AddedAt = e.AddedAt
		   snap.CooldownFull = b.coolOK(e.AddedAt)
		  m := b.loadMeta(ctx, e.UserID, e.Chain, e.Address)
		  if m.Temp {
			  snap.Temp = true
			   snap.TempExpires = m.ExpiresAt
			  if m.expired(b.now()) {
				   snap.Live = false
				   snap.Reason = "temp_expired"
				   if snap.Mode == "" {
					  snap.Mode = EntryModeTemp
				  }
				  return snap
			   }
			   if snap.Mode == "" {
				  snap.Mode = EntryModeTemp
			  }
			  snap.Live = true
		  } else if snap.Mode == "" {
			   if snap.CooldownFull {
				  snap.Mode = EntryModeStrict
				  snap.Live = true
			   } else {
				   snap.Mode = EntryModeCooldownGap
				   snap.Live = false
				   snap.Reason = "cooldown"
			  }
		  }
	  }
	   if !snap.InBook && !snap.GlassLive {
		   snap.Reason = "missing"
		   snap.Live = false
		  return snap
	   }
	  if snap.GlassLive && !snap.InBook {
		  snap.CooldownFull = true
	   }
	   return snap
  }

// ListTempMetas 列出 KV 中的临时覆盖（只读，供过期扫描泵）。

  func (b *Book) ListTempMetas(ctx context.Context, limit int) []EntryMeta {
	  if b == nil || b.kv == nil {
		  return nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   if limit <= 0 {
		  limit = 400
	  }
	  rows, err := b.kv.List(ctx, kindDestMeta, limit)
	   if err != nil {
		   return nil
	   }
	   out := make([]EntryMeta, 0, len(rows))
	  for _, row := range rows {
		   var m EntryMeta
		   if jsonUnmarshalMeta(row.Body, &m) {
			   out = append(out, m)
		   }
	  }
	   return out
   }

// ListBreakGlass 列出旁路记录（只读）。
  func (b *Book) ListBreakGlass(ctx context.Context, limit int) []BreakGlass {
	  if b == nil || b.kv == nil {
		  return nil
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  if limit <= 0 {
		  limit = 400
	  }
	  rows, err := b.kv.List(ctx, kindBreakGlass, limit)
	   if err != nil {
		   return nil
	  }
	  out := make([]BreakGlass, 0, len(rows))
	  for _, row := range rows {
		  var g BreakGlass
		   if jsonUnmarshalGlass(row.Body, &g) {
			   out = append(out, g)
		  }
	  }
	  return out
  }

  func jsonUnmarshalMeta(body []byte, m *EntryMeta) bool {
	  if m == nil || len(body) == 0 {
		  return false
	   }
	   return json.Unmarshal(body, m) == nil
  }

  func jsonUnmarshalGlass(body []byte, g *BreakGlass) bool {
	   if g == nil || len(body) == 0 {
		   return false
	  }
	   return json.Unmarshal(body, g) == nil
   }
