package addrpool

import (
	"context"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

// QuarantineItem 隔离队列一条。

type QuarantineItem struct {
	Address string `json:"address"`
	Chain  string `json:"chain"`
	UserID string `json:"user_id,omitempty"`
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
	Status string `json:"status"`
	Since time.Time `json:"since"`
	Until time.Time `json:"until,omitempty"`
}

// QuarantinePage 隔离队列分页 DTO。
type QuarantinePage struct {
	Items []QuarantineItem `json:"items"`
	Total int              `json:"totalNum"`
	Page int `json:"currentPage"`
	PageSize int `json:"pageSize"`
	Pages    int `json:"totalPage"`
}

  func (b *RecycleBook) List(ctx context.Context, limit int) []RecycleRec {
	   if b == nil || b.kv == nil {
		  return nil
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   if limit <= 0 || limit > 400 {
		  limit = 200
	   }
	  rows, err := b.kv.List(ctx, kindSlot, limit)
	  if err != nil {
		   return nil
	  }
	  out := make([]RecycleRec, 0, len(rows))
	   for _, row := range rows {
		   rec, err := decodeRecycle(row.Body)
		   if err != nil {
			  continue
		   }
		   rec.Fence = row.Fence
		  out = append(out, rec)
	  }
	  return out
  }

   func (m *MarkBook) List() []AddrMark {
	  if m == nil {
		   return nil
	  }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   out := make([]AddrMark, 0, len(m.rows))
	   for _, v := range m.rows {
		  out = append(out, v)
	  }
	   return out
  }

// InspectQuarantine 组装隔离队列 DTO。黑洞先于冷却。

   func InspectQuarantine(marks *MarkBook, rec *RecycleBook, chain string, page, size int) QuarantinePage {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  items := make([]QuarantineItem, 0)
	  seen := map[string]struct{}{}
	   if marks != nil {
		   for _, mk := range marks.List() {
			  if chain != "" && strings.ToUpper(mk.Chain) != chain {
				   continue
			   }
			   key := strings.ToLower(mk.Address)
			  seen[key] = struct{}{}
			   items = append(items, QuarantineItem{
				   Address: mk.Address, Chain: mk.Chain, Kind: mk.Kind,
				   Reason: mk.Reason, Status: "marked", Since: mk.CreatedAt,
			   })
		   }
	   }
	  if rec != nil {
		  for _, slot := range rec.List(context.Background(), 400) {
			   if chain != "" && strings.ToUpper(slot.Chain) != chain {
				  continue
			  }
			   key := strings.ToLower(slot.Address)
			   if _, ok := seen[key]; ok {
				   continue
			   }
			  status := "cooling"
			  if slot.Reclaimed {
				   status = "reclaimed"
			  }
			   items = append(items, QuarantineItem{
				   Address: slot.Address, Chain: slot.Chain, UserID: slot.UserID,
				   Kind: "recycle", Status: status, Since: slot.CoolUntil.Add(-24 * time.Hour),
				  Until: slot.CoolUntil,
			  })
		  }
	  }
	  sort.Slice(items, func(i, j int) bool {
		   if items[i].Since.Equal(items[j].Since) {
			  return items[i].Address < items[j].Address
		   }
		  return items[i].Since.After(items[j].Since)
	  })
	   page = validate.Page(page)
	  size = validate.Limit(size, 50, 200)
	   total := len(items)
	  pages := (total + size - 1) / size
	  if pages < 1 {
		   pages = 1
	   }
	   start := (page - 1) * size
	  if start >= total {
		  return QuarantinePage{Items: []QuarantineItem{}, Total: total, Page: page, PageSize: size, Pages: pages}
	  }
	  end := start + size
	  if end > total {
		   end = total
	  }
	   return QuarantinePage{Items: items[start:end], Total: total, Page: page, PageSize: size, Pages: pages}
  }
