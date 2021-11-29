package subkey

import (
	"errors"
	"strings"
	"time"
)

const (
	defaultRateLimit  = 30
	defaultRateWindow = time.Second
)

var ErrRate = errors.New("api key call rate exceeded")

// RatePolicy 单 Key 滑动窗口；Max<=0 或 Window<=0 表示关闭。

type RatePolicy struct {
	Max    int
	Window time.Duration
}

// BindRatePolicy 注入限流；可运行时调整。
  func (s *Store) BindRatePolicy(p RatePolicy) *Store {
	   if s == nil {
		   return s
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  s.rateMax = p.Max
	   s.rateWin = p.Window
	  if s.hits == nil {
		   s.hits = map[string][]int64{}
	  }
	   return s
  }

  func (s *Store) ratePolicy(rowMax int) (int, time.Duration) {
	  if rowMax > 0 {
		   win := defaultRateWindow
		   if s != nil && s.rateWin > 0 {
			   win = s.rateWin
		  }
		  return rowMax, win
	  }
	  if s != nil && s.rateMax < 0 {
		  return 0, 0 // 显式关闭
	   }
	   max, win := defaultRateLimit, defaultRateWindow
	   if s != nil {
		   if s.rateMax > 0 {
			   max = s.rateMax
		  }
		   if s.rateWin > 0 {
			   win = s.rateWin
		   }
	  }
	   return max, win
   }

// allowRateLocked 调用方须已持 s.mu。窗口内按权重记账；超阈返回 ErrRate；Max<=0 放行。

   func (s *Store) allowRateLocked(apiKey string, rowMax, weight int, now time.Time) error {
	  if s == nil {
		  return ErrGone
	   }
	  max, win := s.ratePolicy(rowMax)
	  if max <= 0 || win <= 0 {
		  return nil
	  }
	  apiKey = strings.TrimSpace(apiKey)
	   if apiKey == "" {
		   return ErrKey
	   }
	   weight = NormalizeWeight(weight)
	  if s.hits == nil {
		  s.hits = map[string][]int64{}
	   }
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  cutoff := now.Add(-win).UnixNano()
	  arr := s.hits[apiKey]
	  kept := arr[:0]
	  for _, t := range arr {
		  if t > cutoff {
			   kept = append(kept, t)
		   }
	  }
	   if len(kept)+weight > max {
		   if len(kept) == 0 {
			  delete(s.hits, apiKey)
		   } else {
			  s.hits[apiKey] = kept
		   }
		   return ErrRate
	  }
	   ts := now.UnixNano()
	  for i := 0; i < weight; i++ {
		   kept = append(kept, ts)
	  }
	  s.hits[apiKey] = kept
	  return nil
   }

// Usage 母户查询单 Key 调用计数与限流/日配额策略；不回显 secret。
  func (s *Store) Usage(owner, subName, apiKey string) (Record, RatePolicy, DayPolicy, error) {
	  if s == nil {
		  return Record{}, RatePolicy{}, DayPolicy{}, ErrGone
	  }
	   owner = strings.TrimSpace(owner)
	  subName = strings.TrimSpace(subName)
	   apiKey = strings.TrimSpace(apiKey)
	  if owner == "" {
		   return Record{}, RatePolicy{}, DayPolicy{}, ErrOwner
	  }
	   if subName == "" {
		   return Record{}, RatePolicy{}, DayPolicy{}, ErrSubName
	  }
	   if apiKey == "" {
		  return Record{},
			  RatePolicy{}, DayPolicy{}, ErrKey
	  }
	  if err := s.assertParent(owner, subName); err != nil {
		  return Record{}, RatePolicy{}, DayPolicy{}, err
	   }
	   s.mu.Lock()
	   r, ok := s.by[apiKey]
	   if !ok || r.Owner != owner || r.SubName != subName {
		  s.mu.Unlock()
		  return Record{},
			   RatePolicy{}, DayPolicy{}, ErrGone
	  }
	  out := r.Record
	  max, win := s.ratePolicy(r.RateMax)
	   day := s.dayPolicy(r.DayCap)
	  s.mu.Unlock()
	   return out, RatePolicy{Max: max, Window: win}, DayPolicy{Max: day}, nil
  }
