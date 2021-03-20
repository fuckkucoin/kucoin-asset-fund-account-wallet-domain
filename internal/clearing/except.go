package clearing

import (
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type ExStatus string

const (
	ExOpen     ExStatus = "open"
	ExResolved ExStatus = "resolved"
	ExRejected ExStatus = "rejected"
)

type Exception struct {
	ID string `json:"id"`
	ItemID  string   `json:"item_id"`
	BatchID string   `json:"batch_id"`
	Reason  string   `json:"reason"`
	Status  ExStatus `json:"status"`
	Actor string        `json:"actor"`
	Note  string        `json:"note,omitempty"`
	Fence persist.Fence `json:"fence"`
	At    time.Time     `json:"at"`
}

// Park 把释放不了的条目送进异常队列。关批前必须处理完 pending，异常可以带着关。
   func (b *Book) Park(itemID, actor, reason string) (Exception, error) {
	   if b == nil {
		  return Exception{}, ErrUnavailable
	   }
	  act, err := actorOK(actor)
	  if err != nil {
		   return Exception{}, err
	  }
	   reason, err = validate.Reason(reason)
	   if err != nil {
		   return Exception{}, err
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  it, ok := b.items[strings.TrimSpace(itemID)]
	   if !ok {
		   return Exception{}, ErrNotFound
	   }
	   if it.Status == StatusReleased || it.Status == StatusClosed {
		  return Exception{}, ErrState
	   }
	  it.Status = StatusExcepted
	  ex := Exception{
		  ID: uuid.NewString(),
		  ItemID: it.ID, BatchID: it.BatchID,
		  Reason: reason, Status: ExOpen, Actor: act, At: b.clock(),
	   }
	   f, err := b.fence.Bump(ex.ID, persist.NewToken())
	   if err != nil {
		  return Exception{}, err
	  }
	  ex.Fence = f
	   cp := ex
	  b.ex[ex.ID] = &cp
	  return ex, nil
  }

  func (b *Book) ResolveEx(id, actor, note string, epoch int64, token string) (Exception, error) {
	  return b.markEx(id, actor, note, epoch, token, ExResolved)
   }

   func (b *Book) RejectEx(id, actor, note string, epoch int64, token string) (Exception, error) {
	  return b.markEx(id, actor, note, epoch, token, ExRejected)
   }

   func (b *Book) markEx(id, actor, note string, epoch int64, token string, to ExStatus) (Exception, error) {
	  act, err := actorOK(actor)
	   if err != nil {
		   return Exception{}, err
	   }
	  if err := b.fence.Check(id, epoch, token); err != nil {
		  return Exception{}, err
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  ex, ok := b.ex[strings.TrimSpace(id)]
	  if !ok {
		  return Exception{}, ErrNotFound
	   }
	  if ex.Status != ExOpen {
		  return *ex, ErrClosed
	   }
	  f, err := b.fence.Bump(id, persist.NewToken())
	  if err != nil {
		  return Exception{}, err
	   }
	   ex.Status = to
	  ex.Actor = act
	   ex.Note = strings.TrimSpace(note)
	  ex.Fence = f
	  return *ex, nil
   }

   func (b *Book) Exceptions(batchID string) []Exception {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Exception, 0)
	   for _, ex := range b.ex {
		   if batchID != "" && ex.BatchID != batchID {
			  continue
		   }
		   out = append(out, *ex)
	  }
	   return out
  }

   func (b *Book) OpenExceptions() []Exception {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Exception, 0)
	  for _, ex := range b.ex {
		   if ex.Status == ExOpen {
			   out = append(out, *ex)
		   }
	  }
	  return out
   }
