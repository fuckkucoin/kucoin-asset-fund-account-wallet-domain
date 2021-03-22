package clearing

import (
	"time"
)

const (
	T0 = 0
	T1 = 1
	T2 = 2
)

// ValueDate 成交日加 T+N 个日历日。周末跳过时只跳 Sat/Sun，不是交易所日历。
   func ValueDate(trade time.Time, n int, skipWeekend bool) (time.Time, error) {
	  if n < 0 || n > 10 {
		   return time.Time{}, ErrOffset
	   }
	   if trade.IsZero() {
		   return time.Time{}, ErrOffset
	   }
	   d := trade.UTC().Truncate(24 * time.Hour)
	   left := n
	  for left > 0 {
		  d = d.Add(24 * time.Hour)
		  if skipWeekend && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
			   continue
		   }
		   left--
	   }
	   return d, nil
   }

   func Ready(value time.Time, now time.Time) bool {
	   if value.IsZero() {
		   return false
	  }
	   return !now.UTC().Before(value.UTC())
   }

  func ParseOffset(n int) (int, error) {
	   if n < 0 || n > 10 {
		   return 0, ErrOffset
	   }
	  return n, nil
   }
