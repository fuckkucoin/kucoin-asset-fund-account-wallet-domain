package shardholdgen

import "time"

  func defaultIssued() time.Time {
	  return time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
  }

// Clock 可注入时钟，巡检与 TTL 单测使用。
type Clock struct {
	now time.Time
}

  func FixedClock(t time.Time) *Clock {
	   return &Clock{now: t.UTC()}
  }

   func (c *Clock) Now() time.Time {
	   if c == nil {
		   return time.Now().UTC()
	  }
	  return c.now
  }

   func (c *Clock) Advance(d time.Duration) {
	  if c != nil {
		   c.now = c.now.Add(d)
	  }
   }
