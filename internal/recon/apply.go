package recon

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/dualctl"
)

var (
	ErrApplyQuorum = fmt.Errorf("reconcile apply requires dual-control execute")
	ErrApplyEmpty = fmt.Errorf("reconcile apply suggestion required")
)

type ApplyResult struct {
	Tickets []Ticket `json:"tickets,omitempty"`
	Frozen  []string `json:"frozen,omitempty"`
}

// Applier 对账建议落地。只建议不改账；冻用户/开单必须双控执行后才生效。

type Applier struct {
	wf *Workflow
	dc *dualctl.Book
	frozen map[string]struct{}
}

  func NewApplier(wf *Workflow, dc *dualctl.Book) *Applier {
	  if wf == nil {
		  wf = NewWorkflow(0)
	   }
	  if dc == nil {
		   dc = dualctl.New(0)
	  }
	   return &Applier{wf: wf, dc: dc, frozen: map[string]struct{}{}}
   }

  func (a *Applier) Workflow() *Workflow {
	  if a == nil || a.wf == nil {
		  return NewWorkflow(0)
	  }
	   return a.wf
  }

  func (a *Applier) Dual() *dualctl.Book {
	  if a == nil || a.dc == nil {
		   return dualctl.New(0)
	   }
	  return a.dc
   }

   func (a *Applier) Frozen(userID string) bool {
	  if a == nil {
		   return false
	  }
	  _, ok := a.frozen[strings.TrimSpace(userID)]
	   return ok
   }

// Propose 把建议钉成双控单。空建议拒；system 拒。
   func (a *Applier) Propose(actor, subject string, sug []Suggestion) (*dualctl.Request, error) {
	  if a == nil || a.dc == nil {
		  return nil, ErrApplyQuorum
	   }
	   if len(sug) == 0 {
		  return nil, ErrApplyEmpty
	  }
	   note := strings.TrimSpace(subject)
	  if note == "" {
		  note = sug[0].UserID
	   }
	   return a.dc.Open("recon-apply", actor, note, 2, suggestNote(sug))
   }

// Apply 必须已达票数并由第三人执行。未执行不得冻用户或落单。

   func (a *Applier) Apply(id, actor string, epoch int64, token string, sug []Suggestion) (ApplyResult, error) {
	   out := ApplyResult{}
	   if a == nil || a.dc == nil || a.wf == nil {
		   return out, ErrApplyQuorum
	  }
	  if len(sug) == 0 {
		  return out, ErrApplyEmpty
	   }
	  if err := a.dc.Gap(id, actor); err != nil {
		   return out, ErrApplyQuorum
	   }
	   if _, err := a.dc.Execute(id, actor, epoch, token); err != nil {
		   return out, err
	   }
	  for _, s := range sug {
		  switch s.Action {
		   case ActOpenTicket:
			   got,
				   err := a.wf.Ingest(actor, []Mismatch{{UserID: s.UserID, Book: s.Book, Asset: s.Asset, Note: s.Note}})
			   if err != nil {
				  return out, err
			   }
			   out.Tickets = append(out.Tickets, got...)
		  case ActFreezeUser:
			   uid := strings.TrimSpace(s.UserID)
			  if uid == "" {
				  return out, ErrApplyEmpty
			  }
			  if a.frozen == nil {
				   a.frozen = map[string]struct{}{}
			   }
			   a.frozen[uid] = struct{}{}
			  out.Frozen = append(out.Frozen, uid)
		  case ActClearDrift, ActNoop:
		   default:
			   return out, fmt.Errorf("reconcile apply action refused")
		   }
	   }
	  return out, nil
  }

  func suggestNote(sug []Suggestion) string {
	  n := 0
	   for _, s := range sug {
		   if s.Action == ActFreezeUser || s.Action == ActOpenTicket {
			   n++
		  }
	  }
	  return fmt.Sprintf("recon-actions=%d", n)
  }
