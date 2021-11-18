package recon

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

var (
	ErrFourEye = fmt.Errorf("reconcile resolve requires a distinct closer")
	ErrSLA = fmt.Errorf("reconcile ticket past SLA remains open")
	ErrReopen = fmt.Errorf("reconcile reopen requires an open mismatch")
)

type Workflow struct {
	book *Book
	sla  time.Duration
}

   func NewWorkflow(sla time.Duration) *Workflow {
	  if sla <= 0 {
		   sla = 4 * time.Hour
	   }
	   return &Workflow{book: New(), sla: sla}
  }

// WrapBook 复用已有对账工单簿（如 Durable.Recon）供 FreezeGate / DriftBlocksFreeze。
  func WrapBook(book *Book, sla time.Duration) *Workflow {
	  if book == nil {
		  return NewWorkflow(sla)
	   }
	  if sla <= 0 {
		   sla = 4 * time.Hour
	   }
	  return &Workflow{book: book, sla: sla}
   }

  func (w *Workflow) Book() *Book {
	   if w == nil || w.book == nil {
		   return New()
	   }
	   return w.book
  }

// Ingest 把对账差异落成工单。空差异不落单。
  func (w *Workflow) Ingest(actor string, mm []Mismatch) ([]Ticket, error) {
	  if w == nil {
		  return nil, ErrNotFound
	   }
	   if len(mm) == 0 {
		   return nil, nil
	  }
	  return w.book.OpenFrom(actor, mm)
  }

// Assign 把未关工单派给处理人。存储故障失败即关。

  func (w *Workflow) Assign(id, actor, assignee string, epoch int64, token string) (*Ticket, error) {
	  if w == nil {
		  return nil, ErrNotFound
	  }
	   return w.book.Assign(id, actor, assignee, epoch, token)
  }

// CloseSafe 解决工单。开单人不得结案；围栏不对失败即关。
  func (w *Workflow) CloseSafe(id, actor string, epoch int64, token string) (*Ticket, error) {
	  if w == nil {
		   return nil, ErrNotFound
	  }
	   cur, err := w.book.Get(id)
	   if err != nil {
		   return nil, err
	  }
	  if strings.EqualFold(cur.OpenedBy, strings.TrimSpace(actor)) {
		   return nil, ErrFourEye
	   }
	  if cur.State == StateOpen {
		   cur, err = w.book.Ack(id, actor, epoch, token)
		   if err != nil {
			   return nil, err
		   }
		  epoch,
			   token = cur.Fence.Epoch, cur.Fence.Token
	   }
	  return w.book.Resolve(cur.ID, actor, epoch, token)
  }

// Breach 超过 SLA 仍未关的单据。不得自动改状态。
  func (w *Workflow) Breach(now time.Time) []Ticket {
	  if w == nil || w.book == nil {
		  return nil
	   }
	  all := w.book.List("", 200)
	   out := make([]Ticket, 0)
	   for _, t := range all {
		  if t.State == StateResolved || t.State == StateRejected {
			   continue
		   }
		  if now.Sub(t.CreatedAt) >= w.sla {
			   out = append(out, t)
		  }
	  }
	   return out
  }

// BlocksUser 该用户仍有未关差异时挡新冻账。
  func (w *Workflow) BlocksUser(userID string) bool {
	  if w == nil {
		  return false
	   }
	  return w.book.OpenCount(userID) > 0
  }

   func (w *Workflow) FenceOf(id string) (persist.Fence, error) {
	  t, err := w.book.Get(id)
	   if err != nil {
		   return persist.Fence{}, err
	  }
	  return t.Fence, nil
  }
