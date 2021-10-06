package quote

import "time"

// Clock 提供 Confirm 判定所用时刻。

type Clock interface {
	Now() time.Time
}

// FixedClock 固定时刻，单测与回放用。
type FixedClock struct {
	T time.Time
}

  func (c FixedClock) Now() time.Time {
	  if c.T.IsZero() {
		  return time.Time{}
	   }
	  return c.T.UTC()
  }

// FuncClock 函数时钟。

type FuncClock func() time.Time

  func (f FuncClock) Now() time.Time {
	   if f == nil {
		   return time.Time{}
	   }
	  return f().UTC()
   }

// StepClock 可步进时钟，用于「过期前后连续 Confirm」。

type StepClock struct {
	Base time.Time
	Off time.Duration
}

  func (c *StepClock) Now() time.Time {
	   if c == nil {
		   return time.Time{}
	   }
	  return c.Base.UTC().Add(c.Off)
  }

// Advance 前进一步。
  func (c *StepClock) Advance(d time.Duration) {
	  if c == nil {
		   return
	  }
	   c.Off += d
   }

// ResolveNow 解析注入时刻：clock 先，否则 now 参数，二者皆空则报错。
  func ResolveNow(clock Clock, now time.Time) (time.Time, error) {
	  if clock != nil {
		  t := clock.Now()
		   if t.IsZero() {
			   return time.Time{}, ErrClockRequired
		  }
		   return t.UTC(), nil
	   }
	   if now.IsZero() {
		  return time.Time{}, ErrClockRequired
	  }
	  return now.UTC(), nil
   }
