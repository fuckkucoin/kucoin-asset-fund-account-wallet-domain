package clearing

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

type BatchStatus string

const (
	BatchOpen BatchStatus = "open"
	BatchClosed BatchStatus = "closed"
)

type Batch struct {
	ID string `json:"id"`
	Day string `json:"day"`
	Status   BatchStatus   `json:"status"`
	OpenedBy string        `json:"opened_by"`
	ClosedBy string        `json:"closed_by,omitempty"`
	OpenedAt time.Time     `json:"opened_at"`
	ClosedAt time.Time     `json:"closed_at,omitempty"`
	Fence    persist.Fence `json:"fence"`
	Pending int `json:"pending"`
	Released int `json:"released"`
	Excepted int `json:"excepted"`
}

  func (b *Book) OpenBatch(day, actor string) (Batch, error) {
	  if b == nil {
		  return Batch{}, ErrUnavailable
	   }
	  act, err := actorOK(actor)
	   if err != nil {
		   return Batch{}, err
	  }
	  day = strings.TrimSpace(day)
	  if day == "" {
		   day = b.clock().Format("2006-01-02")
	  }
	   if _, err := time.Parse("2006-01-02", day); err != nil {
		   return Batch{}, ErrBatch
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   for _, cur := range b.batches {
		   if cur.Day == day && cur.Status == BatchOpen {
			   return *cur, nil
		  }
	   }
	   bat := Batch{ID: uuid.NewString(),
		  Day: day, Status: BatchOpen, OpenedBy: act, OpenedAt: b.clock()}
	   f, err := b.fence.Bump(bat.ID, persist.NewToken())
	   if err != nil {
		  return Batch{}, err
	   }
	   bat.Fence = f
	   cp := bat
	  b.batches[bat.ID] = &cp
	   return bat, nil
  }

   func (b *Book) GetBatch(id string) (Batch, error) {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   bat,
		  ok := b.batches[strings.TrimSpace(id)]
	   if !ok {
		  return Batch{}, ErrNotFound
	  }
	  b.fillCountsLocked(bat)
	  return *bat, nil
  }

  func (b *Book) fillCountsLocked(bat *Batch) {
	  p, r, e := 0, 0, 0
	  for _, id := range b.byBatch[bat.ID] {
		  it := b.items[id]
		   if it == nil {
			   continue
		   }
		  switch it.Status {
		  case StatusPending, StatusReady:
			  p++
		  case StatusReleased:
			  r++
		  case StatusExcepted:
			  e++
		  }
	   }
	  bat.Pending, bat.Released, bat.Excepted = p, r, e
   }

// CloseBatch 未释放且未进异常队列的条目还在时不得关批。
   func (b *Book) CloseBatch(id, actor string, epoch int64, token string) (Batch, error) {
	  if b == nil {
		   return Batch{}, ErrUnavailable
	   }
	  act, err := actorOK(actor)
	  if err != nil {
		   return Batch{}, err
	  }
	   if err := b.fence.Check(id, epoch, token); err != nil {
		  return Batch{}, err
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  bat, ok := b.batches[strings.TrimSpace(id)]
	   if !ok {
		   return Batch{}, ErrNotFound
	  }
	  if bat.Status == BatchClosed {
		   return *bat, ErrClosed
	  }
	   b.fillCountsLocked(bat)
	  if bat.Pending > 0 {
		   return *bat, ErrState
	  }
	  f, err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		  return Batch{}, err
	   }
	  bat.Status = BatchClosed
	   bat.ClosedBy = act
	  bat.ClosedAt = b.clock()
	   bat.Fence = f
	   return *bat, nil
  }
