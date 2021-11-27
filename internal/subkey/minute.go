package subkey

import (
	"errors"
	"strings"
	"time"
)

const defaultMinuteCap = 600

var ErrMinute = errors.New("api key minute call rate exceeded")

// BindMinutePolicy 注入分钟窗上限。

  func (s *Store) BindMinutePolicy(max int) *Store {
	  if s == nil {
		   return s
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   s.minMax = max
	  if s.minHits == nil {
		  s.minHits = map[string][]int64{}
	  }
	  return s
   }

// MinutePolicy 只读分钟窗上限；供用量面展示。
   func (s *Store) MinutePolicy(rowMax int) int {
	   return s.minuteCap(rowMax)
   }

  func (s *Store) minuteCap(rowMax int) int {
	   if rowMax > 0 {
		  return rowMax * 60
	  }
	  if s != nil && s.minMax < 0 {
		  return 0
	   }
	   if s != nil && s.minMax > 0 {
		   return s.minMax
	  }
	  return defaultMinuteCap
  }

// allowMinuteLocked 调用方须已持 s.mu。
  func (s *Store) allowMinuteLocked(apiKey string, rowMax, weight int, now time.Time) error {
	  if s == nil {
		   return ErrGone
	  }
	  max := s.minuteCap(rowMax)
	  if max <= 0 {
		  return nil
	  }
	   apiKey = strings.TrimSpace(apiKey)
	  if apiKey == "" {
		  return ErrKey
	  }
	  weight = NormalizeWeight(weight)
	  if s.minHits == nil {
		  s.minHits = map[string][]int64{}
	   }
	   if now.IsZero() {
		   now = time.Now().UTC()
	  }
	   cutoff := now.Add(-time.Minute).UnixNano()
	   arr := s.minHits[apiKey]
	  kept := arr[:0]
	   for _, t := range arr {
		   if t > cutoff {
			   kept = append(kept, t)
		   }
	   }
	  if len(kept)+weight > max {
		  if len(kept) == 0 {
			   delete(s.minHits, apiKey)
		  } else {
			  s.minHits[apiKey] = kept
		   }
		  return ErrMinute
	   }
	   ts := now.UnixNano()
	   for i := 0; i < weight; i++ {
		  kept = append(kept, ts)
	   }
	  s.minHits[apiKey] = kept
	   return nil
   }
