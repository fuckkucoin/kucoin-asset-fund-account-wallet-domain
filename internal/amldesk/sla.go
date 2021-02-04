package amldesk

import (
	"encoding/json"
	"time"
)

const DefaultSLA = 24 * time.Hour

type SLA struct {
	CaseID string `json:"case_id"`
	Status Status `json:"status"`
	Age   time.Duration `json:"age"`
	Limit time.Duration `json:"limit"`
	Overdue bool `json:"overdue"`
	Opened time.Time `json:"opened_at"`
}

// ScanSLA 只看未结案。超时不得自动关，只能标 overdue 或 escalate。
   func (b *Book) ScanSLA(now time.Time, limit time.Duration) []SLA {
	   if b == nil {
		  return nil
	  }
	  if limit <= 0 {
		   limit = DefaultSLA
	   }
	   if now.IsZero() {
		  now = b.clock()
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]SLA, 0)
	  for _, r := range b.rows {
		  if !r.Open() {
			  continue
		  }
		  age := now.Sub(r.OpenedAt)
		   out = append(out, SLA{
			  CaseID: r.ID, Status: r.Status, Age: age, Limit: limit,
			   Overdue: age > limit, Opened: r.OpenedAt,
		   })
	   }
	   return out
   }

  func (b *Book) Overdue(now time.Time, limit time.Duration) []SLA {
	  rows := b.ScanSLA(now, limit)
	   out := make([]SLA, 0, len(rows))
	   for _, r := range rows {
		  if r.Overdue {
			   out = append(out, r)
		  }
	  }
	   return out
   }

// EscalateOverdue 把超时未结案升到 escalated。开案人自己不得借 SLA 销案。

   func (b *Book) EscalateOverdue(actor string, now time.Time, limit time.Duration) ([]Record, error) {
	  if b == nil {
		   return nil, ErrUnavailable
	  }
	  act, err := actorOK(actor)
	   if err != nil {
		  return nil, ErrActor
	  }
	   over := b.Overdue(now, limit)
	  out := make([]Record, 0)
	   for _, row := range over {
		   cur, err := b.Get(row.CaseID)
		   if err != nil {
			   continue
		   }
		  if cur.Status == StatusEscalated {
			   continue
		   }
		  got, err := b.mark(cur.ID, act, StatusEscalated, cur.Fence.Epoch, cur.Fence.Token, "sla overdue")
		  if err != nil {
			   return out, err
		  }
		  out = append(out, got)
	  }
	  return out, nil
   }

  func (b *Book) Now() time.Time {
	  if b == nil || b.clock == nil {
		  return time.Now().UTC()
	   }
	  return b.clock()
   }

// QueueOverdue 超时未结案队列。只读，不改行。
  func (b *Book) QueueOverdue(now time.Time, limit time.Duration) []Record {
	  over := b.Overdue(now, limit)
	  out := make([]Record, 0, len(over))
	   for _, row := range over {
		   if rec, err := b.Get(row.CaseID); err == nil {
			  out = append(out, rec)
		  }
	  }
	   return out
   }

  func (b *Book) ExportOverdue(now time.Time, limit time.Duration) ([]byte, error) {
	  if b == nil {
		   return nil, ErrUnavailable
	  }
	  rows := b.QueueOverdue(now, limit)
	  out := DeskExport{Count: len(rows), Cases: rows, ExportedAt: time.Now().UTC()}
	   for _, r := range rows {
		   if r.Open() {
			   out.Open++
		   }
		   out.Timeline = append(out.Timeline, b.Timeline(r.ID)...)
	  }
	  return json.Marshal(out)
   }
