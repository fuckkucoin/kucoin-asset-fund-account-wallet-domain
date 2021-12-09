package sweeppol

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
)

var (
	ErrHaltActor  = fmt.Errorf("sweep halt actor invalid")
	ErrHaltReview = fmt.Errorf("sweep halt lift requires a distinct reviewer")
)

// Controller 归集停机闸。停机单人即可；解除必须换人复核。
type Controller struct {
	mu sync.Mutex
	book *Book
	sch  Schedule
	by   string
	lift string
}

  func NewController(book *Book, sch Schedule) *Controller {
	  if book == nil {
		  book = New(nil)
	   }
	   return &Controller{book: book, sch: sch}
   }

  func haltActor(actor string) error {
	  if strings.TrimSpace(actor) == "" || strings.EqualFold(actor, "system") {
		  return ErrHaltActor
	   }
	  return nil
  }

  func (c *Controller) Halt(actor string) error {
	   if c == nil {
		  return ErrHaltActor
	  }
	  if err := haltActor(actor); err != nil {
		   return err
	   }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	   c.sch.Halt = true
	  c.by = strings.TrimSpace(actor)
	  c.lift = ""
	   return nil
  }

   func (c *Controller) Lift(actor, reviewer string) error {
	  if c == nil {
		   return ErrHaltActor
	  }
	   if err := haltActor(actor); err != nil {
		  return err
	  }
	  if err := haltActor(reviewer); err != nil {
		  return ErrHaltReview
	   }
	  if strings.EqualFold(strings.TrimSpace(actor), strings.TrimSpace(reviewer)) {
		  return ErrHaltReview
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	   if c.sch.Halt && strings.EqualFold(c.by, strings.TrimSpace(reviewer)) && strings.EqualFold(c.by, strings.TrimSpace(actor)) {
		  return ErrHaltReview
	  }
	   c.sch.Halt = false
	  c.lift = strings.TrimSpace(reviewer)
	   return nil
  }

  func (c *Controller) Snapshot() Schedule {
	  if c == nil {
		  return DefaultSchedule()
	  }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  return c.sch
  }

  func (c *Controller) PlanAt(inv []hotinv.Snapshot, now time.Time) ([]Plan, error) {
	   if c == nil || c.book == nil {
		   return nil, fmt.Errorf("sweep controller unavailable")
	   }
	  c.mu.Lock()
	  sch := c.sch
	   book := c.book
	   c.mu.Unlock()
	   return book.PlanAt(inv, sch, now)
   }
