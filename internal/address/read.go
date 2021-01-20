package address

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type BookQuery struct {
	UserID  string
	Chain   string
	Address string
	Currency string
	Asset string
	Tag   string
	Page int
	PageSize int
	IncludeDisabled bool
}

type BookItem struct {
	ledger.AddressBookEntry
	Nickname string `json:"nickname,omitempty"`
	Tag      string `json:"tag,omitempty"`
	FormatOK bool `json:"format_ok"`
	FormatErr string                `json:"format_error,omitempty"`
	Format    wallet.AddrFormatSpec `json:"format"`
	SoftDeleted bool      `json:"soft_deleted,omitempty"`
	LastUsedAt  time.Time `json:"last_used_at,omitempty"`
	DupChains   []string  `json:"dup_chains,omitempty"`
}

type BookPage struct {
	Entries []BookItem `json:"entries"`
	Total   int        `json:"totalNum"`
	Page int `json:"currentPage"`
	PageSize int `json:"pageSize"`
	TotalPage int    `json:"totalPage"`
	Currency  string `json:"currency,omitempty"`
}

type BookInspect struct {
	Entry ledger.AddressBookEntry `json:"entry"`
	Found bool `json:"found"`
	FormatOK  bool                  `json:"format_ok"`
	FormatErr string                `json:"format_error,omitempty"`
	Format    wallet.AddrFormatSpec `json:"format"`
}

type AllowInspect struct {
	Allowlisted bool `json:"allowlisted"`
	DestAllow bool `json:"destallow"`
	DestAllowExact bool `json:"destallow_exact"`
	WhitelistOnly  bool `json:"whitelist_only"`
	CooldownOK     bool `json:"cooldown_ok"`
	CooldownLeft int64 `json:"cooldown_left_sec"`
	FormatOK     bool  `json:"format_ok"`
	Code string `json:"code,omitempty"`
}

  func addrExact(got, want string) bool {
	  if strings.TrimSpace(want) == "" {
		   return false
	  }
	   return destallow.AddrExact(got, want)
  }

   func chainMatch(entry, want string) bool {
	  return destallow.ChainEqual(entry,
		  want)
   }

// 簿项补形态闸字段。链地址走 wallet.ValidChainAddress。形态失败不得冒充已校验。
  func DecorateBookEntry(e ledger.AddressBookEntry) BookItem {
	   return DecorateBookEntryMeta(e,
		   BookMeta{})
   }

// DecorateBookEntryMeta 簿项补形态闸、昵称与标签。
   func DecorateBookEntryMeta(e ledger.AddressBookEntry, meta BookMeta) BookItem {
	   item := BookItem{AddressBookEntry: e,
		  Format: wallet.FormatSpec(e.Chain), SoftDeleted: e.Disabled}
	   applyMeta(&item, meta, e.Label)
	   if e.Address != "" && e.Chain != "" {
		  if err := wallet.ValidChainAddress(e.Chain, e.Address); err != nil {
			   item.FormatErr = err.Error()
		  } else {
			  item.FormatOK = true
		   }
	  list, err := s.ListBook(ctx, userID)
	  if err != nil {
		  return ledger.AddressBookEntry{}, false, err
	  }
	   addr = strings.TrimSpace(addr)
	  if addr == "" {
		  return ledger.AddressBookEntry{}, false, nil
	   }
	   for _, e := range list {
		  if !addrExact(e.Address, addr) {
			  continue
		   }
		   if chainMatch(e.Chain, chain) {
			   return e, true, nil
		   }
	   }
	  return ledger.AddressBookEntry{}, false, nil
   }

// 过滤后分页列出地址簿。page/pageSize 默认 1/50，硬顶 200；条目带形态闸。分页越界返回空页，不得回上一页数据。
  func (s *Service) ListBookPage(ctx context.Context, q BookQuery) (BookPage, error) {
	  list, err := s.ListBook(ctx, q.UserID)
	   if err != nil {
		   return BookPage{}, err
	   }
	   asset := strings.ToUpper(strings.TrimSpace(q.Asset))
	  if asset == "" {
		   asset = strings.ToUpper(strings.TrimSpace(q.Currency))
	  }
	   tag := strings.TrimSpace(q.Tag)
	   filtered := make([]ledger.AddressBookEntry, 0, len(list))
	   for _, e := range list {
		   if e.Disabled && !q.IncludeDisabled {
			  continue
		   }
		   if q.Chain != "" && !chainMatch(e.Chain, q.Chain) {
			  continue
		  }
		  if q.Address != "" && !addrExact(e.Address, q.Address) {
			  continue
		  }
		  if asset != "" && strings.ToUpper(strings.TrimSpace(e.Asset)) != asset {
			   continue
		   }
		  if tag != "" {
			  meta := BookMeta{}
			  if s != nil {
				  meta = s.bookMeta(ctx, e.UserID, e.Chain, e.Address)
			   }
			   if !strings.EqualFold(strings.TrimSpace(meta.Tag), tag) {
				  continue
			  }
		   }
		   filtered = append(filtered, e)
	   }
	   page := validate.Page(q.Page)
	   size := validate.Limit(q.PageSize, 50, 200)
	  total := len(filtered)
	  pages := (total + size - 1) / size
	  if pages < 1 {
		  pages = 1
	  }
	   cur := strings.ToUpper(strings.TrimSpace(q.Currency))
	   start := (page - 1) * size
	   if start >= total {
		  return BookPage{Entries: []BookItem{},
			   Total: total, Page: page, PageSize: size, TotalPage: pages, Currency: cur}, nil
	  }
	   end := start + size
	   if end > total {
		   end = total
	  }
	  dups, _ := s.DetectCrossChainDups(ctx, q.UserID)
	   dupMap := map[string][]string{}
	  for _, d := range dups {
		   dupMap[d.Address] = d.Chains
	   }
	   items := make([]BookItem, 0, end-start)
	   for _, e := range filtered[start:end] {
		   item := s.decorate(ctx, e)
		   item.DupChains = dupMap[e.Address]
		   items = append(items, item)
	   }
	  return BookPage{Entries: items, Total: total, Page: page, PageSize: size, TotalPage: pages, Currency: cur}, nil
   }

// 读单条簿项并过形态闸。不写簿；未命中仍校验请求地址。形态失败须显式回 FormatErr。
  func (s *Service) InspectBook(ctx context.Context, userID, chain, addr string) (BookInspect, error) {
	   e, ok, err := s.GetBook(ctx, userID, chain, addr)
	  if err != nil {
		  return BookInspect{}, err
	   }
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  if ok && e.Chain != "" {
		  ch = e.Chain
	   }
	   item := s.decorate(ctx, e)
	  if !ok {
		  item = DecorateBookEntry(ledger.AddressBookEntry{Chain: ch, Address: strings.TrimSpace(addr)})
	   }
	  return BookInspect{Entry: e, Found: ok, FormatOK: item.FormatOK, FormatErr: item.FormatErr, Format: item.Format}, nil
   }

// 读白名单闸。簿命中走 260325 全等；destallow 另报折叠/精确/冷却。WhitelistOnly 开启且簿未命中须回 260325。

   func (s *Service) InspectAllow(ctx context.Context, userID, chain, addr string, book *destallow.Book, pref destallow.PrefReader) (AllowInspect, error) {
	   ok, err := s.Allowlisted(ctx, userID, chain, addr)
	  if err != nil {
		  return AllowInspect{}, err
	  }
	   view := AllowInspect{Allowlisted: ok, CooldownOK: true}
	   if book != nil {
		   g := book.InspectGate(ctx, pref, userID, chain, addr)
		  view.DestAllow = g.DestAllow
		  view.DestAllowExact = g.DestAllowExact
		   view.CooldownOK = g.CooldownOK
		   view.CooldownLeft = g.CooldownLeft
		   view.FormatOK = g.FormatOK
		  view.WhitelistOnly = g.WhitelistOnly
	   } else if pref != nil {
		  view.WhitelistOnly = pref.WhitelistOnly(ctx,
			  userID)
	  }
	  if !view.FormatOK && strings.TrimSpace(addr) != "" && strings.TrimSpace(chain) != "" {
		   view.FormatOK = wallet.ValidChainAddress(strings.ToUpper(strings.TrimSpace(chain)), strings.TrimSpace(addr)) == nil
	   }
	   if view.WhitelistOnly && !ok {
		  view.Code = "260325"
	  }
	   return view, nil
  }

// 只读取已分配充值地址。不 Allocate / Ensure；FamMEMO 回填 memo。未登记共享地址不得种子派生。

  func (s *Service) InspectDeposit(userID, chain, asset, memo string) addrpool.AssignedView {
	   if s == nil {
		  return addrpool.AssignedView{}
	   }
	  view := addrpool.InspectAssigned(s.pool, s.shared, userID, chain, asset, memo)
	   return addrpool.FillAssignedMemo(userID, []addrpool.AssignedView{view}, fundsurf.MemoFor)[0]
  }

// 列出用户已分配充值地址。含 FamMEMO 共享面；备注按用户+链回填。不得现场 Ensure 共享地址。

  func (s *Service) InspectDepositList(userID, chain, asset, memo string) []addrpool.AssignedView {
	   if s == nil {
		   return nil
	  }
	  return addrpool.FillAssignedMemo(userID, addrpool.InspectAssignedList(s.pool, s.shared, userID, chain, asset, memo), fundsurf.MemoFor)
  }

// 地址簿条数。与 ListBookPage 同一过滤。不得另开无过滤计数。
   func (s *Service) BookCount(ctx context.Context, userID, chain string) (int, error) {
	  p, err := s.ListBookPage(ctx, BookQuery{UserID: userID, Chain: chain, Page: 1, PageSize: 1})
	  if err != nil {
		   return 0, err
	  }
	  return p.Total, nil
   }

// 已分配充值地址分页。复用 InspectDepositList 后再切页。越界空页。

  func (s *Service) InspectDepositPage(userID, chain, asset string, page, size int) (items []addrpool.AssignedView, total, pageOut, sizeOut, pages int) {
	   return addrpool.PageAssigned(s.InspectDepositList(userID, chain, asset, ""), page, size)
  }

// 按充值地址反查归属。走池 ByAddress，不 Allocate。未绑 Found=false。
  func (s *Service) InspectDepositByAddress(addr string) addrpool.AssignedView {
	   if s == nil {
		   return addrpool.AssignedView{}
	  }
	   return addrpool.InspectByAddress(s.pool, addr)
   }
