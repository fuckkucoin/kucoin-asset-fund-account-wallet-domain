package destallow

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type PrefReader interface {
	WhitelistOnly(ctx context.Context, userID string) bool
}

type GateView struct {
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Address        string `json:"address"`
	DestAllow      bool   `json:"destallow"`
	DestAllowExact bool   `json:"destallow_exact"`
	WhitelistOnly  bool   `json:"whitelist_only"`
	CooldownOK     bool   `json:"cooldown_ok"`
	CooldownLeft int64 `json:"cooldown_left_sec"`
	FormatOK     bool  `json:"format_ok"`
	FormatError string `json:"format_error,omitempty"`
}

type EntryView struct {
	Entry
	FormatOK    bool   `json:"format_ok"`
	FormatError string `json:"format_error,omitempty"`
	CooldownOK bool `json:"cooldown_ok"`
	CooldownLeft int64 `json:"cooldown_left_sec"`
	RiskTag   string    `json:"risk_tag,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Temp      bool      `json:"temp,omitempty"`
	Expired bool `json:"expired,omitempty"`
}

   func clipEntries(in []Entry, page, size int) (out []Entry, total, pageOut, sizeOut, pages int) {
	  page = validate.Page(page)
	  size = validate.Limit(size, 50, 200)
	  total = len(in)
	  pages = (total + size - 1) / size
	  if pages < 1 {
		  pages = 1
	   }
	   start := (page - 1) * size
	   if start >= total {
		   return []Entry{},
			  total, page, size, pages
	  }
	  end := start + size
	   if end > total {
		   end = total
	   }
	  return in[start:end], total, page, size, pages
   }

// 簿项补形态闸。地址走 wallet.ValidChainAddress。形态失败不得标 format_ok。

   func DecorateEntry(e Entry) EntryView {
	   view := EntryView{Entry: e}
	   if e.Address != "" && e.Chain != "" {
		   if err := wallet.ValidChainAddress(e.Chain, e.Address); err != nil {
			   view.FormatError = err.Error()
		   } else {
			  view.FormatOK = true
		   }
	   }
	   return view
  }

  func (b *Book) coolLeft(added time.Time) int64 {
	  if b == nil || b.cooldown <= 0 || added.IsZero() {
		   return 0
	  }
	  now := time.Now().UTC()
	  if b.clock != nil {
		  now = b.clock()
	  }
	  until := added.Add(b.cooldown)
	   if !until.After(now) {
		   return 0
	  }
	  return int64(until.Sub(now) / time.Second)
   }

// 条目补形态闸与冷却剩余。冷却秒数取自 Book.cooldown。冷却未满不得当已放行。

  func (b *Book) ViewOf(e Entry) EntryView {
	  view := DecorateEntry(e)
	  if b == nil {
		   view.CooldownOK = true
		  return view
	   }
	   view.CooldownOK = b.coolOK(e.AddedAt)
	   view.CooldownLeft = b.coolLeft(e.AddedAt)
	   view.applyMeta(b.loadMeta(context.Background(), e.UserID, e.Chain, e.Address), b.now)
	  return view
   }

// 分页条目补形态闸与冷却。与 ListPage 同序。空页返回空切片。

  func (b *Book) DecorateEntries(in []Entry) []EntryView {
	   out := make([]EntryView, 0, len(in))
	  for _, e := range in {
		   out = append(out, b.ViewOf(e))
	   }
	   return out
  }

// 按存储键读取一条。链大写；地址键可折叠。存储故障当未命中，不得伪装放行。
   func (b *Book) Get(userID, chain, addr string) (Entry, bool) {
	   e,
		  ok, err := b.lookup(userID, chain, addr)
	   if err != nil || !ok {
		   return Entry{}, false
	   }
	   return e, true
   }

// 按地址全等取一条。大小写敏感；EVM 族互认。折叠命中不算精确。
   func (b *Book) GetExact(userID, chain, addr string) (Entry, bool) {
	  if b == nil {
		   return Entry{}, false
	  }
	  addr = strings.TrimSpace(addr)
	   if addr == "" {
		   return Entry{}, false
	  }
	   for _, e := range b.List(userID) {
		  if !AddrExact(e.Address, addr) {
			  continue
		  }
		   if ChainEqual(e.Chain, chain) {
			  return e, true
		  }
	   }
	   return Entry{}, false
   }

// 判定目标是否精确在白名单。地址大小写敏感全等。不得用折叠键放行。

   func (b *Book) AllowedExact(userID, chain, addr string) bool {
	  _, ok := b.GetExact(userID, chain, addr)
	  return ok
  }

// 按用户（可选链、标签）列出白名单。链/标签空则不过滤该维。簿空返回 nil。

   func (b *Book) ListFilter(userID, chain, label string) []Entry {
	   if b == nil {
		   return nil
	  }
	   all := b.List(userID)
	   label = strings.TrimSpace(label)
	  out := make([]Entry, 0, len(all))
	  for _, e := range all {
		   if chain != "" && !ChainEqual(e.Chain, chain) {
			  continue
		  }
		   if label != "" && e.Label != label {
			  continue
		  }
		   out = append(out, e)
	  }
	   return out
   }

// 白名单分页读。page/pageSize 硬顶同地址簿；可选 label。越界空页。

  func (b *Book) ListPage(userID, chain string, page, size int, label string) (items []Entry, total, pageOut, sizeOut, pages int) {
	  return clipEntries(b.ListFilter(userID, chain, label), page, size)
  }

// 组合 WhitelistOnly 与目标是否允许。附带形态闸与冷却可读。冷却未满 cooldown_ok=false，不得当已放行。

  func (b *Book) InspectGate(ctx context.Context, pref PrefReader, userID, chain, addr string) GateView {
	   view := GateView{
		   UserID: strings.TrimSpace(userID),
		  Chain:   NormChain(chain),
		  Address: strings.TrimSpace(addr),
		  CooldownOK: true,
	  }
	   if b != nil {
		  if e, ok := foldHit(b, userID, view.Chain, view.Address); ok {
			  view.DestAllow = b.coolOK(e.AddedAt)
			   view.CooldownOK = view.DestAllow
			  view.CooldownLeft = b.coolLeft(e.AddedAt)
		   }
		  view.DestAllowExact = b.AllowedExact(userID, view.Chain, view.Address)
		  if e, ok := b.GetExact(userID, view.Chain, view.Address); ok {
			   view.CooldownOK = b.coolOK(e.AddedAt)
			   view.CooldownLeft = b.coolLeft(e.AddedAt)
		   }
	  }
	  if pref != nil {
		  view.WhitelistOnly = pref.WhitelistOnly(ctx, userID)
	  }
	   if view.Chain != "" && view.Address != "" {
		   if err := wallet.ValidChainAddress(view.Chain, view.Address); err != nil {
			  view.FormatError = err.Error()
		  } else {
			   view.FormatOK = true
		   }
	   }
	   return view
  }
