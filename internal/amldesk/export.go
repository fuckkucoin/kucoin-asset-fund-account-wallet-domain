package amldesk

import (
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type DeskExport struct {
	UserID string `json:"user_id,omitempty"`
	Count int `json:"count"`
	Open  int `json:"open"`
	Closed int      `json:"closed"`
	Cases  []Record `json:"cases"`
	Timeline   []Event   `json:"timeline,omitempty"`
	ExportedAt time.Time `json:"exported_at"`
}

// Export 导出案卷 JSON。只读，不改行。
  func (b *Book) Export(userID string) ([]byte, error) {
	  if b == nil {
		   return nil, ErrUnavailable
	  }
	  rows := b.List(strings.TrimSpace(userID), 10000)
	  out := DeskExport{UserID: strings.TrimSpace(userID), Count: len(rows), Cases: rows, ExportedAt: time.Now().UTC()}
	  for _, r := range rows {
		   if r.Open() {
			   out.Open++
		  }
		  if r.Status == StatusClosed || r.Status == StatusRejected {
			   out.Closed++
		  }
		  out.Timeline = append(out.Timeline, b.Timeline(r.ID)...)
	   }
	   return json.Marshal(out)
  }

// Reopen 仅 rejected 可重开。已 closed 的案不得改写，只能再开新案。

  func (b *Book) Reopen(id, actor string, epoch int64, token, note string) (Record, error) {
	   if b == nil {
		   return Record{}, ErrUnavailable
	   }
	   act, err := actorOK(actor)
	   if err != nil {
		  return Record{}, ErrActor
	  }
	   cur, err := b.Get(id)
	  if err != nil {
		  return Record{}, err
	  }
	   if cur.Status == StatusClosed {
		  return cur, ErrClosed
	  }
	  if cur.Status != StatusRejected {
		   return cur, ErrState
	  }
	   if strings.EqualFold(act, cur.OpenedBy) {
		   return cur, ErrFourEye
	  }
	   if err := b.fence.Check(id, epoch, token); err != nil {
		  return Record{}, err
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   r, ok := b.rows[id]
	  if !ok {
		  return Record{}, ErrNotFound
	  }
	  f, err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return Record{}, err
	   }
	  r.Status = StatusOpen
	   r.ClosedBy = ""
	  r.ClosedAt = time.Time{}
	   r.Note = strings.TrimSpace(note)
	  r.Fence = f
	   b.noteLocked(id, act, "reopened", note)
	  return *r, nil
   }
