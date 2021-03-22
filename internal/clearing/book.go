package clearing

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Book struct {
	mu      sync.Mutex
	batches map[string]*Batch
	items   map[string]*Item
	byBatch map[string][]string
	ex      map[string]*Exception
	applied map[string]bool
	hol    *Calendar
	fence  *persist.Table
	clock  func() time.Time
	skipWE bool
}

  func NewBook() *Book {
	   return &Book{
		   batches: map[string]*Batch{}, items: map[string]*Item{}, byBatch: map[string][]string{},
		  ex:      map[string]*Exception{},
		  applied: map[string]bool{}, hol: NewCalendar(),
		   fence: persist.NewTable(),
		   clock: func() time.Time { return time.Now().UTC() }, skipWE: true,
	   }
  }

   func (b *Book) WithClock(c func() time.Time) *Book {
	  if b != nil && c != nil {
		  b.clock = c
	  }
	   return b
  }

  func (b *Book) SkipWeekend(v bool) *Book {
	  if b != nil {
		   b.skipWE = v
	  }
	   return b
  }

   func actorOK(actor string) (string, error) {
	   a, err := validate.Actor(actor)
	  if err != nil {
		   return "", ErrActor
	   }
	   return a, nil
   }
