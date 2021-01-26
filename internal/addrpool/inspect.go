package addrpool

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type AssignedView struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Asset  string `json:"asset,omitempty"`
	Family string `json:"family"`
	Address string `json:"address"`
	BoundAt time.Time `json:"bound_at,omitempty"`
	Shared bool   `json:"shared"`
	Memo   string `json:"memo,omitempty"`
	Tag string `json:"tag,omitempty"`
	TagName  string `json:"tag_name,omitempty"`
	NeedMemo bool   `json:"needMemo"`
	Format wallet.AddrFormatSpec `json:"format"`
	FormatOK  bool   `json:"format_ok"`
	FormatErr string `json:"format_error,omitempty"`
	Allocated bool `json:"allocated"`
	Found bool `json:"found"`
	ReuseAfter time.Time `json:"reuse_after,omitempty"`
}

type MemoFn func(userID, chain string) string

// 取 Runtime 上的共享池。没有则新建并按需绑 SQL。须运营 Register，不得静默种子。

  func SharedFromRuntime(shared any, sql *persist.SQLDB, mem bool) *SharedMemoPool {
	  if shared != nil {
		  if s, ok := shared.(*SharedMemoPool); ok && s != nil {
			  return s
		   }
	   }
	   s := NewSharedMemoPool(nil)
	  if !mem && sql != nil {
		   s.BindSQL(sql)
	   }
	   return s
  }

// 列出用户在指定链（或家族）上的已绑定地址。空链返回该用户全部。未分配不得现场派生。
  func (p *Pool) ListChain(userID, chain string) []Binding {
	  all := p.List(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   if chain == "" {
		   return all
	   }
	  want := family(chain)
	  out := make([]Binding, 0, len(all))
	  for _, b := range all {
		  if b.Chain == chain || b.Family == want {
			  out = append(out, b)
		   }
	   }
	  return out
  }

  func stampFormat(view *AssignedView) {
	  if view == nil || view.Address == "" || view.Chain == "" {
		   return
	   }
	  if err := wallet.ValidChainAddress(view.Chain, view.Address); err != nil {
		   view.FormatErr = err.Error()
		   return
	   }
	   view.FormatOK = true
  }

// 只读一条已分配地址。FamMEMO 走共享池 Address，不 Ensure；过形态闸。未登记共享面 Found=false。

   func InspectAssigned(pool *Pool, shared *SharedMemoPool, userID, chain, asset, memo string) AssignedView {
	   userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  memo = strings.TrimSpace(memo)
	   view := AssignedView{UserID: userID, Chain: chain, Asset: asset, Format: wallet.FormatSpec(chain)}
	  view.NeedMemo = wallet.RequiresFamMemo(chain)
	  view.TagName = wallet.FamMemoTagName(chain)
	  if wallet.RequiresFamMemo(chain) && shared != nil {
		  addr, ok := shared.Address(chain)
		  if !ok {
			   return view
		  }
		  view.Found = true
		  view.Shared = true
		   view.Address = addr
		   view.Family = "MEMO"
		  view.Allocated = true
		   if memo != "" {
			  view.Memo = memo
			   view.Tag = memo
		   }
		  stampFormat(&view)
		   return view
	  }
	  if pool == nil || userID == "" || chain == "" {
		   return view
	  }
	  b, ok := pool.Lookup(userID, chain)
	   if !ok {
		   return view
	  }
	   view.Found = true
	   view.Family = b.Family
	   view.Address = b.Address
	   view.BoundAt = b.BoundAt
	  view.Allocated = b.Allocated
	  view.ReuseAfter = b.Cooldown
	   stampFormat(&view)
	  return view
  }

// 按已绑地址反查归属。走 Pool.ByAddress，不 Allocate。未绑返回 Found=false。

   func InspectByAddress(pool *Pool, addr string) AssignedView {
	   if pool == nil {
		   return AssignedView{}
	  }
	  b, ok := pool.ByAddress(strings.TrimSpace(addr))
	  if !ok {
		  return AssignedView{Address: strings.TrimSpace(addr)}
	  }
	   view := InspectAssigned(pool, nil, b.UserID, b.Chain, "", "")
	  if !view.Found {
		  view.UserID = b.UserID
		   view.Chain = b.Chain
		   view.Family = b.Family
		   view.Address = b.Address
		  view.BoundAt = b.BoundAt
		   view.ReuseAfter = b.Cooldown
		   view.Allocated = b.Allocated
		   view.Found = true
		   stampFormat(&view)
	   }
	  return view
  }

// 已分配地址分页。page/pageSize 默认 1/50，硬顶 200。越界返回空页。
  func PageAssigned(items []AssignedView, page, size int) (out []AssignedView, total, pageOut, sizeOut, pages int) {
	   page = validate.Page(page)
	   size = validate.Limit(size, 50, 200)
	  total = len(items)
	   pages = (total + size - 1) / size
	  if pages < 1 {
		  pages = 1
	   }
	   start := (page - 1) * size
	   if start >= total {
		  return []AssignedView{}, total, page, size, pages
	  }
	   end := start + size
	   if end > total {
		   end = total
	  }
	   return items[start:end], total, page, size, pages
   }

// 列出已分配地址。指定链返回 0/1 条；空链汇一对一池与已登记共享面。同地址不重复。

   func InspectAssignedList(pool *Pool, shared *SharedMemoPool, userID, chain, asset, memo string) []AssignedView {
	  userID = strings.TrimSpace(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   out := make([]AssignedView, 0)
	  if chain != "" {
		  v := InspectAssigned(pool, shared, userID, chain, asset, memo)
		  if v.Found {
			  out = append(out, v)
		  }
		  return out
	  }
	  seen := map[string]struct{}{}
	   if pool != nil {
		  for _, b := range pool.List(userID) {
			   v := InspectAssigned(pool, shared, userID, b.Chain, asset, memo)
			   if !v.Found {
				   continue
			  }
			   k := v.Chain + "|" + v.Address
			   if _, ok := seen[k]; ok {
				  continue
			  }
			   seen[k] = struct{}{}
			   out = append(out, v)
		  }
	   }
	  if shared != nil {
		  for _, row := range shared.List() {
			   if row.Chain == "" {
				   continue
			   }
			   v := InspectAssigned(pool,
				  shared, userID, row.Chain, asset, memo)
			  if !v.Found {
				   continue
			   }
			   k := v.Chain + "|" + v.Address
			  if _, ok := seen[k]; ok {
				  continue
			  }
			   seen[k] = struct{}{}
			  out = append(out, v)
		   }
	  }
	   return out
  }

// 给已分配面回填 FamMEMO 备注。仅 NeedMemo 且 memo 空时回填。不得编造链上 tag。
// FamAttrView FamMEMO 归因检视。
type FamAttrView struct {
	SharedMemoBinding
	BinderRequired bool `json:"binder_required"`
	Attributed bool `json:"attributed"`
	Registered bool `json:"registered"`
	Missing []string `json:"missing,omitempty"`
}

// InspectFamAttr 只读 FamMEMO 共享面与归因缺口。
  func InspectFamAttr(shared *SharedMemoPool, userID, chain, memo string) FamAttrView {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   view := FamAttrView{
		  SharedMemoBinding: SharedMemoBinding{UserID: strings.TrimSpace(userID), Chain: chain, Memo: strings.TrimSpace(memo), Shared: true},
	   }
	  if shared == nil {
		   view.Missing = []string{chain}
		   return view
	  }
	  view.BinderRequired = shared.BinderRequired()
	   addr, ok := shared.Address(chain)
	   view.Registered = ok
	  view.Address = addr
	   view.TagName = wallet.FamMemoTagName(chain)
	  if !ok {
		   view.Missing = []string{chain}
	   }
	  if strings.TrimSpace(memo) != "" && ok {
		  view.Attributed = wallet.ValidFamMemoTag(chain, memo) == nil
		   view.Tag = strings.TrimSpace(memo)
	  }
	  return view
  }

  func FillAssignedMemo(userID string, items []AssignedView, memoFor MemoFn) []AssignedView {
	   if memoFor == nil || len(items) == 0 {
		  return items
	   }
	  for i := range items {
		  if !items[i].NeedMemo || items[i].Memo != "" {
			  continue
		  }
		  m := strings.TrimSpace(memoFor(userID, items[i].Chain))
		  if m == "" {
			   continue
		  }
		   items[i].Memo = m
		   items[i].Tag = m
	   }
	  return items
   }
