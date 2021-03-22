package clearing

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// MarkReady 价值日已到才标 ready。不到日不得当可释放。
   func (b *Book) MarkReady(id string, now time.Time) (Item, error) {
	  if b == nil {
		   return Item{}, ErrUnavailable
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   it, ok := b.items[strings.TrimSpace(id)]
	  if !ok {
		   return Item{}, ErrNotFound
	  }
	   if it.Status != StatusPending && it.Status != StatusReady {
		   return *it, ErrState
	   }
	   if now.IsZero() {
		   now = b.clock()
	   }
	  if !Ready(it.ValueDate, now) {
		  return *it, ErrEarly
	   }
	  it.Status = StatusReady
	   return *it, nil
  }

   func (b *Book) ScanReady(now time.Time) []Item {
	   if now.IsZero() && b != nil {
		  now = b.clock()
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Item, 0)
	  for _, it := range b.items {
		  if it.Status != StatusPending {
			   continue
		  }
		  if Ready(it.ValueDate, now) {
			  it.Status = StatusReady
			  out = append(out, *it)
		  }
	   }
	  return out
  }

// Release 只释放 ready。本金不在这里改账，调用方拿到列表后再 credit。
   func (b *Book) Release(id, actor string, epoch int64, token string) (Item, error) {
	  if b == nil {
		   return Item{}, ErrUnavailable
	  }
	  if _, err := actorOK(actor); err != nil {
		   return Item{}, err
	  }
	   if err := b.fence.Check(id,
		  epoch, token); err != nil {
		  return Item{}, err
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   it, ok := b.items[strings.TrimSpace(id)]
	  if !ok {
		  return Item{}, ErrNotFound
	  }
	  if it.Status != StatusReady {
		  return *it, ErrState
	   }
	  f, err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return Item{}, err
	   }
	  it.Status = StatusReleased
	  it.Fence = f
	  return *it, nil
  }
