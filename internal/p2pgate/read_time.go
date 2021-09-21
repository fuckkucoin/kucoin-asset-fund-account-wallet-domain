package p2pgate

import (
	"strconv"
	"strings"
	"time"
)

// ParseQueryTime 接受空串、十进制毫秒、十进制秒（10 位）或 RFC3339。

  func ParseQueryTime(raw string) (time.Time, error) {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return time.Time{}, nil
	   }
	  if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		   if n <= 0 {
			   return time.Time{}, ErrQueryRange
		  }
		  if n < 1e11 {
			  return time.Unix(n, 0).UTC(), nil
		  }
		   return time.UnixMilli(n).UTC(),
			  nil
	   }
	  if t, err := time.Parse(time.RFC3339, raw); err == nil {
		   return t.UTC(), nil
	  }
	  if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		   return t.UTC(), nil
	   }
	   // 资金页偶发 "2006-01-02" 日切。
	  if t, err := time.Parse("2006-01-02", raw); err == nil {
		  return t.UTC(), nil
	  }
	  if t, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
		  return t.UTC(), nil
	  }
	   return time.Time{}, ErrQueryRange
  }

// DayKey UTC 日期键。

  func DayKey(t time.Time) string {
	  if t.IsZero() {
		   return ""
	   }
	  return t.UTC().Format("2006-01-02")
  }

// ClampWindow 将查询窗裁到 [now-maxAge, now]。maxAge<=0 时不裁左端。
  func ClampWindow(start, end time.Time, now time.Time, maxAge time.Duration) (time.Time, time.Time) {
	  if now.IsZero() {
		   now = time.Now().UTC()
	  }
	   if end.IsZero() || end.After(now) {
		  end = now
	  }
	  if maxAge > 0 {
		   floor := now.Add(-maxAge)
		   if start.IsZero() || start.Before(floor) {
			   start = floor
		   }
	  }
	   if !start.IsZero() && start.After(end) {
		  start = end
	  }
	   return start, end
   }

// WindowDays 返回闭区间内的 UTC 日期列表，硬顶 366 以免读面爆炸。
  func WindowDays(start, end time.Time) []string {
	   if start.IsZero() || end.IsZero() {
		   return nil
	  }
	   start = start.UTC().Truncate(24 * time.Hour)
	   end = end.UTC().Truncate(24 * time.Hour)
	   if end.Before(start) {
		  return nil
	  }
	   out := make([]string, 0, 8)
	   for d := start; !d.After(end); d = d.Add(24 * time.Hour) {
		   out = append(out, DayKey(d))
		   if len(out) >= 366 {
			  break
		   }
	  }
	   return out
   }

// CreatedAtMs 从占用 CreatedAt 字符串取毫秒；解析失败返回 0。

   func CreatedAtMs(rfc3339 string) int64 {
	  t := parseRFC3339(rfc3339)
	   if t.IsZero() {
		   return 0
	   }
	   return t.UnixMilli()
  }
