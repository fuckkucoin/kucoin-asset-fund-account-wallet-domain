package riskscore

import (
	"strings"
	"time"
)

// WindowOf 按用户/目标切 1h/24h 速度和复用。评估当时冻结，后写事件不改已算窗。
   func (s *Service) WindowOf(userID, chain, dest string, now time.Time) Window {
	   if s == nil {
		  return Window{}
	  }
	  if now.IsZero() {
		  now = s.now()
	   }
	   userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  dest = strings.TrimSpace(dest)
	   h1 := now.Add(-1 * time.Hour)
	  d1 := now.Add(-24 * time.Hour)
	  day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	  s.mu.Lock()
	  evs := append([]Event(nil), s.events...)
	  s.mu.Unlock()
	   var w Window
	  seenDest := map[string]struct{}{}
	   seenToday := map[string]struct{}{}
	  for _, e := range evs {
		   if e.UserID != userID {
			  continue
		  }
		  if !e.At.Before(h1) {
			   w.Velocity1h++
		  }
		   if !e.At.Before(d1) {
			   w.Velocity24h++
			   k := strings.ToUpper(e.Chain) + "|" + e.Dest
			   seenDest[k] = struct{}{}
			  if strings.EqualFold(e.Chain, chain) && e.Dest == dest {
				  w.DestCount24h++
			   }
		   }
		  if !e.At.Before(day) {
			   seenToday[strings.ToUpper(e.Chain)+"|"+e.Dest] = struct{}{}
		  }
		  if strings.EqualFold(e.Chain, chain) && e.Dest == dest {
			  w.DestCount++
		   }
	  }
	   w.NewDests = len(seenToday)
	  if dest != "" {
		  members := s.graph.Correlated(chain, dest)
		   w.ClusterSize = len(members)
		  if w.ClusterSize == 0 {
			   w.ClusterSize = 1
		   }
		   w.Correlated = w.ClusterSize
	  }
	   _ = seenDest
	   return w
  }
