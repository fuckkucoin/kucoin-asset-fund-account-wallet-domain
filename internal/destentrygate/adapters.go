package destentrygate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/destallow"
)

// BookAdapter 只读包装 destallow.Book + PrefReader。
type BookAdapter struct {
	Book *destallow.Book
	Pref destallow.PrefReader
}

// WhitelistOnly 读偏好；pref 为空视为开启（fail-closed）。
   func (a BookAdapter) WhitelistOnly(ctx context.Context, userID string) bool {
	  if a.Pref == nil {
		   return true
	   }
	   return a.Pref.WhitelistOnly(ctx, userID)
   }

// Snapshot 映射 destallow.EntrySnapshot。
  func (a BookAdapter) Snapshot(ctx context.Context, userID, chain, addr string) (LiveEntry, error) {
	  if a.Book == nil {
		  return LiveEntry{Reason: "book_nil"}, ErrUnavailable
	   }
	  s := a.Book.SnapshotEntry(ctx,
		   userID, chain, addr)
	  return mapSnapshot(s), nil
  }

// ListExpiredTemp 扫描已过期临时条。
   func (a BookAdapter) ListExpiredTemp(ctx context.Context, now time.Time, limit int) ([]LiveEntry, error) {
	   if a.Book == nil {
		  return nil, nil
	  }
	  metas := a.Book.ListTempMetas(ctx, limit)
	   out := make([]LiveEntry, 0)
	   for _, m := range metas {
		   if !m.Temp || m.ExpiresAt.IsZero() || m.ExpiresAt.After(now) {
			  continue
		   }
		   out = append(out, LiveEntry{
			  EntryID: destallow.ComputeEntryID(m.UserID,
				   m.Chain, m.Address),
			   UserID:  m.UserID,
			   Chain:   m.Chain,
			  Address: m.Address,
			  Mode:    ModeTemp,
			   Live:   false,
			  Reason: "temp_expired",
			   TempExpires: m.ExpiresAt,
		   })
	  }
	   return out, nil
  }

// ListExpiredGlass 扫描已失效破窗。
   func (a BookAdapter) ListExpiredGlass(ctx context.Context, now time.Time, limit int) ([]LiveEntry, error) {
	   if a.Book == nil {
		  return nil, nil
	   }
	   list := a.Book.ListBreakGlass(ctx,
		  limit)
	  out := make([]LiveEntry, 0)
	  for _, g := range list {
		  if g.ExpiresAt.IsZero() || g.ExpiresAt.After(now) {
			   continue
		   }
		  out = append(out, LiveEntry{
			  EntryID: destallow.ComputeEntryID(g.UserID,
				   g.Chain, g.Address),
			  UserID:  g.UserID,
			   Chain:   g.Chain,
			  Address: g.Address,
			  Mode: ModeBreakglass,
			   Live: false,
			  Reason:       "breakglass_reclaimed",
			   GlassExpires: g.ExpiresAt,
		   })
	   }
	  return out, nil
   }

   func mapSnapshot(s destallow.EntrySnapshot) LiveEntry {
	   mode := ModeStrict
	   switch s.Mode {
	  case destallow.EntryModeTemp:
		   mode = ModeTemp
	   case destallow.EntryModeBreakglass:
		   mode = ModeBreakglass
	   case destallow.EntryModeCooldownGap:
		   mode = ModeStrict
	   }
	   return LiveEntry{
		   EntryID: s.EntryID,
		   UserID:  s.UserID,
		   Chain:   s.Chain,
		  Address: s.Address,
		   Mode:         mode,
		  Live:         s.Live,
		   CooldownFull: s.CooldownFull,
		  Reason:       s.Reason,
		   TempExpires:  s.TempExpires,
		  GlassExpires: s.GlassExpires,
	   }
  }

// StaticPref 用偏好。
type StaticPref struct{ On bool }

   func (p StaticPref) WhitelistOnly(context.Context, string) bool { return p.On }

// MemorySurface 用活条目面。
type MemorySurface struct {
	Only bool
	Entries  map[string]LiveEntry
	ExpiredT []LiveEntry
	ExpiredG []LiveEntry
}

  func NewMemorySurface(only bool) *MemorySurface {
	  return &MemorySurface{Only: only, Entries: map[string]LiveEntry{}}
  }

   func liveKey(userID, chain, addr string) string {
	  return userID + "|" + chain + "|" + addr
   }

  func (m *MemorySurface) Put(e LiveEntry) {
	   if m.Entries == nil {
		   m.Entries = map[string]LiveEntry{}
	  }
	  m.Entries[liveKey(e.UserID, e.Chain, e.Address)] = e
   }

   func (m *MemorySurface) WhitelistOnly(context.Context, string) bool {
	  if m == nil {
		  return true
	   }
	   return m.Only
   }

   func (m *MemorySurface) Snapshot(_ context.Context, userID, chain, addr string) (LiveEntry, error) {
	   if m == nil {
		   return LiveEntry{}, ErrUnavailable
	  }
	   e, ok := m.Entries[liveKey(userID, chain, addr)]
	  if !ok {
		  return LiveEntry{UserID: userID, Chain: chain, Address: addr, Reason: "missing"}, nil
	   }
	  return e, nil
  }

   func (m *MemorySurface) ListExpiredTemp(context.Context, time.Time, int) ([]LiveEntry, error) {
	   if m == nil {
		   return nil, nil
	  }
	  return append([]LiveEntry(nil), m.ExpiredT...), nil
  }

   func (m *MemorySurface) ListExpiredGlass(context.Context, time.Time, int) ([]LiveEntry, error) {
	   if m == nil {
		  return nil, nil
	  }
	  return append([]LiveEntry(nil), m.ExpiredG...), nil
   }

// RecordingSink 记录出金提交调用。

type RecordingSink struct {
	Ref     string
	Calls   int
	LastID  string
	FailErr error
}

   func (r *RecordingSink) Submit(_ context.Context, _, _, _, leaseID, _ string) (string, error) {
	  if r == nil {
		   return "", ErrUnavailable
	   }
	  r.Calls++
	   r.LastID = leaseID
	  if r.FailErr != nil {
		  return "", r.FailErr
	  }
	   if r.Ref == "" {
		   return "wd-ref", nil
	   }
	  return r.Ref, nil
   }

// RejectSink 永远拒绝，用于证明 fail-closed 不旁路。

type RejectSink struct{}

  func (RejectSink) Submit(context.Context, string, string, string, string, string) (string, error) {
	  return "", ErrWithdrawBlocked
   }
