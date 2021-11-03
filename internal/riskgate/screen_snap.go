package riskgate

import "strings"

// ScreenSnap 给筛查租约闸使用的只读投影。
type ScreenSnap struct {
	Halted bool
	Mode         string
	WindowCount  int
	Velocity1h   int
	Velocity24h  int
	DestCount    int
	DestCount24h int
}

// SnapshotForLease 按用户与目的地投影当前闸状态。

   func (s *Service) SnapshotForLease(user, dest string) ScreenSnap {
	  out := ScreenSnap{}
	   if s == nil {
		   return out
	   }
	  out.Halted = s.Halted()
	   out.Mode = string(s.Engine().Mode())
	  wins := s.Windows()
	  out.WindowCount = len(wins)
	  seen := map[string]bool{}
	   seen24 := map[string]bool{}
	   for _, w := range wins {
		   if w.UserID != user {
			  continue
		   }
		   out.Velocity24h++
		   out.Velocity1h++
		  if strings.TrimSpace(w.Dest) != "" {
			   seen[w.Dest] = true
			   seen24[w.Dest] = true
		   }
	   }
	   if dest != "" {
		  seen[dest] = true
		   seen24[dest] = true
	   }
	  out.DestCount = len(seen)
	  out.DestCount24h = len(seen24)
	  return out
  }
