package subkey

import (
	"errors"
	"time"
)

const defaultDayCap = 100_000

var ErrDay = errors.New("api key daily call quota exceeded")

// DayPolicy 日配额；Max<0 关闭，Max==0 用默认。

type DayPolicy struct {
	Max int
}

// BindDayPolicy 注入日配额策略。

  func (s *Store) BindDayPolicy(p DayPolicy) *Store {
	  if s == nil {
		  return s
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  s.dayMax = p.Max
	  return s
  }

   func (s *Store) dayPolicy(rowCap int) int {
	   if rowCap > 0 {
		  return rowCap
	   }
	  if s != nil && s.dayMax < 0 {
		   return 0 // 显式关闭
	   }
	   if s != nil && s.dayMax > 0 {
		  return s.dayMax
	  }
	  return defaultDayCap
  }

  func dayStampUTC(t time.Time) int {
	  if t.IsZero() {
		   t = time.Now().UTC()
	  } else {
		  t = t.UTC()
	   }
	   y, m, d := t.Date()
	   return y*10000 + int(m)*100 + d
  }

// allowDayLocked 调用方须已持 s.mu。跨日清零；按权重累加，超阈 ErrDay。

  func (s *Store) allowDayLocked(r *row, now time.Time, weight int) error {
	  if r == nil {
		  return ErrGone
	   }
	  capN := s.dayPolicy(r.DayCap)
	   if capN <= 0 {
		  return nil
	   }
	   weight = NormalizeWeight(weight)
	  stamp := dayStampUTC(now)
	  if r.DayStamp != stamp {
		   r.DayStamp = stamp
		   r.DayCount = 0
	  }
	  if r.DayCount+int64(weight) > int64(capN) {
		  return ErrDay
	  }
	  r.DayCount += int64(weight)
	   return nil
  }
