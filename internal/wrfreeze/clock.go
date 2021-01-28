package wrfreeze

import "time"

// NowClock 返回当前 UTC 时间（默认时钟）。
   func NowClock() time.Time { return time.Now().UTC() }

// StaticClock 返回偏移 n 小时的静态时钟函数（负值表示过去）。
  func StaticClock(hoursOffset int) func() time.Time {
	  t := time.Now().UTC().Add(time.Duration(hoursOffset) * time.Hour)
	   return func() time.Time { return t }
  }
