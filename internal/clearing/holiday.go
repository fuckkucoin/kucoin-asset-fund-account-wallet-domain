package clearing

import (
	"strings"
	"sync"
	"time"
)

// Calendar 额外休市日。周末之外再跳这些日期，不是交易所官方日历。
type Calendar struct {
	mu  sync.Mutex
	off map[string]struct{}
}

   func NewCalendar() *Calendar {
	  return &Calendar{off: map[string]struct{}{}}
   }

  func dayKey(t time.Time) string {
	   return t.UTC().Format("2006-01-02")
  }

   func (c *Calendar) Add(day string) error {
	  if c == nil {
		   return ErrUnavailable
	  }
	   day = strings.TrimSpace(day)
	   if _, err := time.Parse("2006-01-02", day); err != nil {
		  return ErrHoliday
	   }
	   c.mu.Lock()
	   c.off[day] = struct{}{}
	   c.mu.Unlock()
	  return nil
  }

   func (c *Calendar) Remove(day string) {
	  if c == nil {
		   return
	   }
	   c.mu.Lock()
	   delete(c.off,
		  strings.TrimSpace(day))
	   c.mu.Unlock()
  }

  func (c *Calendar) Has(t time.Time) bool {
	  if c == nil {
		  return false
	   }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	   _, ok := c.off[dayKey(t)]
	  return ok
   }

  func (c *Calendar) List() []string {
	   if c == nil {
		  return nil
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	  out := make([]string, 0, len(c.off))
	   for d := range c.off {
		   out = append(out, d)
	   }
	   return out
  }

   func (b *Book) Holidays() *Calendar {
	  if b == nil {
		  return NewCalendar()
	   }
	   if b.hol == nil {
		   b.hol = NewCalendar()
	  }
	  return b.hol
  }

  func (b *Book) WithHolidays(c *Calendar) *Book {
	  if b != nil && c != nil {
		   b.hol = c
	  }
	   return b
  }

// ValueDateCal 成交日加 T+N 个营业日。周末和休市日都不计数。
  func ValueDateCal(trade time.Time, n int, skipWeekend bool, hol *Calendar) (time.Time, error) {
	  if n < 0 || n > 10 {
		  return time.Time{}, ErrOffset
	   }
	  if trade.IsZero() {
		   return time.Time{}, ErrOffset
	  }
	  d := trade.UTC().Truncate(24 * time.Hour)
	  if n == 0 {
		  return d, nil
	  }
	   left := n
	   for left > 0 {
		  d = d.Add(24 * time.Hour)
		   if skipWeekend && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
			  continue
		  }
		  if hol != nil && hol.Has(d) {
			   continue
		  }
		  left--
	  }
	   return d, nil
  }
