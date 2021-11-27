package subkey

import (
	"errors"
	"strings"
	"time"
)

const defaultOwnerRate = 200

var ErrOwnerRate = errors.New("owner api key call rate exceeded")

// BindOwnerRate 注入母户聚合限流。
   func (s *Store) BindOwnerRate(max int) *Store {
	   if s == nil {
		   return s
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  s.ownerRate = max
	  if s.ownerHits == nil {
		   s.ownerHits = map[string][]int64{}
	  }
	  return s
   }

// OwnerRatePolicy 只读母户聚合上限；供用量面展示。

  func (s *Store) OwnerRatePolicy() int {
	  return s.ownerRateMax()
  }

  func (s *Store) ownerRateMax() int {
	  if s != nil && s.ownerRate < 0 {
		   return 0
	   }
	   if s != nil && s.ownerRate > 0 {
		   return s.ownerRate
	   }
	  return defaultOwnerRate
  }

// allowOwnerRateLocked 调用方须已持 s.mu。按权重记账。
  func (s *Store) allowOwnerRateLocked(owner string, weight int, now time.Time) error {
	  if s == nil {
		  return ErrGone
	   }
	  max := s.ownerRateMax()
	  if max <= 0 {
		   return nil
	  }
	  owner = strings.TrimSpace(owner)
	  if owner == "" {
		   return ErrOwner
	  }
	  weight = NormalizeWeight(weight)
	   if s.ownerHits == nil {
		  s.ownerHits = map[string][]int64{}
	   }
	  if now.IsZero() {
		  now = time.Now().UTC()
	  }
	   win := defaultRateWindow
	   if s.rateWin > 0 {
		   win = s.rateWin
	   }
	  cutoff := now.Add(-win).UnixNano()
	   arr := s.ownerHits[owner]
	  kept := arr[:0]
	  for _, t := range arr {
		   if t > cutoff {
			   kept = append(kept, t)
		  }
	  }
	   if len(kept)+weight > max {
		   if len(kept) == 0 {
			  delete(s.ownerHits, owner)
		   } else {
			   s.ownerHits[owner] = kept
		   }
		  return ErrOwnerRate
	  }
	   ts := now.UnixNano()
	  for i := 0; i < weight; i++ {
		  kept = append(kept, ts)
	  }
	   s.ownerHits[owner] = kept
	  return nil
  }
