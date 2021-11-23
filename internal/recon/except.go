package recon

import "strings"

// Exception 对账异常分类。只描述差异，不改账。

type Exception struct {
	Kind string `json:"kind"`
	Book string `json:"book,omitempty"`
	Asset string `json:"asset,omitempty"`
	Note string `json:"note"`
}

const (
	ExMismatch = "mismatch"
	ExMissingLeg = "missing_leg"
)

// KindOf 按 Delta.Note 分类。含 missing 视为缺腿。

   func KindOf(d Delta) string {
	  if strings.Contains(strings.ToLower(d.Note), "missing") {
		   return ExMissingLeg
	  }
	   return ExMismatch
  }

// Exceptions 把对账 Delta 编成异常清单。OK 时为空。
  func Exceptions(rep CompareReport) []Exception {
	   if rep.OK {
		   return nil
	   }
	   out := make([]Exception,
		  0, len(rep.Deltas))
	  for _, d := range rep.Deltas {
		   out = append(out, Exception{Kind: KindOf(d), Book: d.Book, Asset: d.Asset, Note: d.Note})
	   }
	   return out
  }

// Replay 已结案且差异仍在才重开。已一致不得重开。
  func (w *Workflow) Replay(id, actor string, epoch int64, token string, still CompareReport) (*Ticket, error) {
	   if still.OK {
		   return nil, ErrReopen
	   }
	   return w.Reopen(id,
		  actor, epoch, token, true)
   }
