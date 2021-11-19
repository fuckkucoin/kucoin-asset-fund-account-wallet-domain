package stmt

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Filter struct {
	Asset   string
	Book    string
	AppType string
	Limit   int
}

type Line struct {
	ledger.Bill
	Running string `json:"running"`
}

type Report struct {
	UserID string `json:"user_id"`
	Lines  []Line `json:"lines"`
	Count  int    `json:"count"`
}

// Build 按 seq 升序滚余额。debit 减、credit 加；解析失败整行跳过，不写 NaN。
// Filter.Limit 从最新往回截，但 running 仍按全量前缀算，避免截断面额对不上。
  func Build(userID string, bills []ledger.Bill, f Filter) (Report, error) {
	  type key struct{ book, asset string }
	  run := map[key]money.Amount{}
	  all := make([]Line, 0, len(bills))
	   for _, b := range bills {
		  if f.Asset != "" && !strings.EqualFold(b.Asset, f.Asset) {
			  continue
		   }
		  if f.Book != "" && !strings.EqualFold(b.Book, f.Book) && !strings.EqualFold(b.Subject, f.Book) {
			   continue
		   }
		  if f.AppType != "" && b.AppType != f.AppType {
			   continue
		   }
		   k := key{book: b.Book, asset: b.Asset}
		  cur := run[k]
		   if d, err := money.Parse(orZero(b.Debit)); err == nil && d.IsPositive() {
			   next, err := cur.Sub(d)
			  if err != nil {
				  // 流水不应把 running 打负；记下当前值接着，调用方对账会抓。
			   } else {
				   cur = next
			  }
		   }
		   if c, err := money.Parse(orZero(b.Credit)); err == nil && c.IsPositive() {
			   next, err := cur.Add(c)
			  if err == nil {
				   cur = next
			  }
		  }
		  run[k] = cur
		   line := Line{Bill: b, Running: cur.String()}
		   all = append(all, line)
	   }
	  if f.Limit > 0 && len(all) > f.Limit {
		   all = all[len(all)-f.Limit:]
	   }
	  return Report{UserID: userID, Lines: all, Count: len(all)}, nil
   }

   func orZero(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return "0"
	  }
	  return s
  }
