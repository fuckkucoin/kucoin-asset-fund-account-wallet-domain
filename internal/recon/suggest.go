package recon

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Action string

const (
	ActOpenTicket Action = "open_ticket"
	ActFreezeUser Action = "freeze_user"
	ActClearDrift Action = "clear_drift"
	ActNoop Action = "noop"
)

type Suggestion struct {
	Action Action `json:"action"`
	UserID string `json:"user_id"`
	Book   string `json:"book,omitempty"`
	Asset string `json:"asset,omitempty"`
	Note  string `json:"note"`
}

// Suggest 只出建议，不改账。可用对不上先冻用户；只冻结差建议开单。
   func Suggest(rep CompareReport) []Suggestion {
	   if rep.OK {
		   return []Suggestion{{Action: ActClearDrift, UserID: rep.UserID, Note: "sources agree"}}
	  }
	  out := make([]Suggestion, 0, len(rep.Deltas)+1)
	   needFreeze := false
	  for _, d := range rep.Deltas {
		  left, lerr := money.ParseBook(orZ(d.Left))
		  right, rerr := money.ParseBook(orZ(d.Right))
		  note := d.Note
		   if lerr != nil || rerr != nil {
			   note = "unparsable amount"
			  needFreeze = true
		  } else if strings.Contains(d.Note, "available") || left.Cmp(right) != 0 {
			  needFreeze = true
		  }
		   out = append(out, Suggestion{Action: ActOpenTicket, UserID: rep.UserID, Book: d.Book, Asset: d.Asset, Note: note})
	   }
	  if needFreeze {
		   out = append(out, Suggestion{Action: ActFreezeUser, UserID: rep.UserID, Note: "freeze-on-drift"})
	   }
	  return out
  }

  func ToMismatches(rep CompareReport) []Mismatch {
	  out := make([]Mismatch, 0, len(rep.Deltas))
	   for _, d := range rep.Deltas {
		   out = append(out, Mismatch{UserID: rep.UserID, Book: d.Book, Asset: d.Asset, Note: d.Note})
	   }
	   return out
  }
