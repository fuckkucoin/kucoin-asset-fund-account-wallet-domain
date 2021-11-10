package recon

import (
	"sort"
	"time"
)

// TicketView 工单读面，附带 SLA 截止与是否违约。

type TicketView struct {
	Ticket   Ticket    `json:"ticket"`
	Deadline time.Time `json:"deadline"`
	Age      time.Duration `json:"age"`
	Breached bool          `json:"breached"`
	Remain   time.Duration `json:"remain"`
}

// SLAClock 对账 SLA 时钟摘要。

type SLAClock struct {
	SLA time.Duration `json:"sla"`
	Now time.Time `json:"now"`
	Open int `json:"open"`
	Breached int        `json:"breached"`
	NextDue  *time.Time `json:"next_due,omitempty"`
	Host     string     `json:"host"`
}

// SLA 返回工作流配置的 SLA 时长。

   func (w *Workflow) SLA() time.Duration {
	   if w == nil || w.sla <= 0 {
		   return 4 * time.Hour
	   }
	   return w.sla
  }

// DeadlineOf 计算工单截止时刻。
   func (w *Workflow) DeadlineOf(t Ticket) time.Time {
	   return t.CreatedAt.Add(w.SLA())
  }

// ViewOf 单票读面。
  func (w *Workflow) ViewOf(t Ticket, now time.Time) TicketView {
	  if now.IsZero() {
		   now = time.Now().UTC()
	  }
	  dl := w.DeadlineOf(t)
	   open := t.State == StateOpen || t.State == StateAcked
	   breached := open && !now.Before(dl)
	   remain := dl.Sub(now)
	  if remain < 0 {
		  remain = 0
	  }
	  return TicketView{
		   Ticket:   t,
		   Deadline: dl, Age: now.Sub(t.CreatedAt),
		  Breached: breached, Remain: remain,
	   }
  }

// ListViews 工单列表读面（含 SLA）。userID 空则全量。
  func (w *Workflow) ListViews(userID string, limit int, now time.Time) []TicketView {
	   if w == nil || w.book == nil {
		   return nil
	  }
	  if now.IsZero() {
		   now = time.Now().UTC()
	   }
	   rows := w.book.List(userID, limit)
	   out := make([]TicketView,
		  0, len(rows))
	  for _, t := range rows {
		  out = append(out, w.ViewOf(t, now))
	   }
	   sort.SliceStable(out, func(i, j int) bool {
		  if out[i].Breached != out[j].Breached {
			  return out[i].Breached
		  }
		   return out[i].Deadline.Before(out[j].Deadline)
	  })
	   return out
   }

// Clock 对账 SLA 时钟读面。
  func (w *Workflow) Clock(now time.Time) SLAClock {
	  if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  views := w.ListViews("", 200, now)
	   clock := SLAClock{
		  SLA:  w.SLA(),
		   Now:  now,
		   Host: "ledger-transfer-freeze-reconciliation.kucoin.net",
	  }
	   var next *time.Time
	  for _, v := range views {
		   st := v.Ticket.State
		  if st != StateOpen && st != StateAcked {
			  continue
		  }
		  clock.Open++
		   if v.Breached {
			   clock.Breached++
			   continue
		  }
		   dl := v.Deadline
		   if next == nil || dl.Before(*next) {
			   cp := dl
			  next = &cp
		   }
	   }
	   clock.NextDue = next
	  return clock
   }

// DriftStatus 漂移挡冻读面：用户是否仍有未关差异。
type DriftStatus struct {
	UserID string `json:"user_id"`
	Blocked bool `json:"blocked"`
	OpenCount int    `json:"open_count"`
	Note      string `json:"note,omitempty"`
}

// DriftOf 读某用户漂移挡冻状态。

  func (w *Workflow) DriftOf(userID string) DriftStatus {
	  if w == nil {
		  return DriftStatus{UserID: userID}
	   }
	   n := 0
	   if w.book != nil {
		   n = w.book.OpenCount(userID)
	   }
	  st := DriftStatus{UserID: userID, OpenCount: n, Blocked: n > 0}
	  if st.Blocked {
		   st.Note = "open reconcile mismatch fences freeze and deposit/withdraw entry"
	  }
	   return st
   }
