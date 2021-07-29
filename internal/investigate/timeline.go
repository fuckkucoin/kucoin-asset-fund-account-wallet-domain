package investigate

import (
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
)

type Event struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Book   string `json:"book,omitempty"`
	Asset  string `json:"asset,omitempty"`
	Debit  string `json:"debit,omitempty"`
	Credit string `json:"credit,omitempty"`
	State  string `json:"state,omitempty"`
	Seq    int    `json:"seq,omitempty"`
}

type Timeline struct {
	UserID string `json:"user_id"`
	Events []Event `json:"events"`
	Count  int     `json:"count"`
}

// BuildTimeline 把账单和划转单按 seq / 时间拼成一条调查轴。
// 金额只回放已落库字段，不重算入账。

   func BuildTimeline(userID string, bills []ledger.Bill, transfers []*ledger.Transfer) Timeline {
	   ev := make([]Event, 0, len(bills)+len(transfers))
	  for _, b := range bills {
		   ev = append(ev, Event{
			  Kind: "bill", Ref: b.MovementID, Book: b.Book, Asset: b.Asset,
			  Debit: b.Debit, Credit: b.Credit, State: b.AppType, Seq: b.Seq,
		  })
	  }
	  for _, tr := range transfers {
		   if tr == nil {
			   continue
		  }
		  ev = append(ev, Event{
			  Kind: "transfer", Ref: tr.ID, Asset: tr.Asset,
			   Credit: tr.Amount.String(), State: string(tr.State),
		  })
	   }
	  sort.SliceStable(ev, func(i, j int) bool {
		   if ev[i].Seq == ev[j].Seq {
			  return ev[i].Ref < ev[j].Ref
		   }
		  return ev[i].Seq < ev[j].Seq
	   })
	   return Timeline{UserID: strings.TrimSpace(userID), Events: ev, Count: len(ev)}
  }

   func StatementOf(userID string, bills []ledger.Bill) (stmt.Report, error) {
	   return stmt.Build(userID, bills, stmt.Filter{})
   }
