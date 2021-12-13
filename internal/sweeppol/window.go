package sweeppol

import "time"

// WindowLabel 给人看的窗口说明。相等表示全天。

  func WindowLabel(start, end int) string {
	   if start == end {
		   return "all-day"
	  }
	  if start > end {
		   return "overnight"
	  }
	  return "same-day"
  }

// HoursOpen 列出 UTC 小时里哪些点落在窗口内，供夹具。
  func HoursOpen(start, end int) []int {
	   out := make([]int, 0, 24)
	  r := RebalanceRule{WindowStart: start,
		   WindowEnd: end}
	   base := time.Date(2022, 5, 22, 0, 0, 0, 0, time.UTC)
	  for h := 0; h < 24; h++ {
		  if r.InWindow(base.Add(time.Duration(h) * time.Hour)) {
			   out = append(out, h)
		  }
	   }
	  return out
   }

// NextOpen 给定时刻之后最近的开窗时刻。已经在窗内则返回原时刻。

   func NextOpen(r RebalanceRule, at time.Time) time.Time {
	  at = at.UTC()
	   if r.InWindow(at) {
		  return at
	   }
	  for i := 1; i <= 24; i++ {
		  n := at.Add(time.Duration(i) * time.Hour).Truncate(time.Hour)
		  if r.InWindow(n) {
			   return n
		   }
	  }
	  return at
  }
