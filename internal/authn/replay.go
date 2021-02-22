package authn

import "time"

// PurgeSeen 丢掉超出偏斜窗的 nonce。窗口内复用仍失败即关。
   func (g *Guard) PurgeSeen(now int64) int {
	  if g == nil {
		  return 0
	  }
	   g.mu.Lock()
	   defer g.mu.Unlock()
	   if g.seen == nil {
		   return 0
	  }
	   keep := int64(g.skew / time.Second)
	   if keep <= 0 {
		  keep = 300
	  }
	  n := 0
	   for k, ts := range g.seen {
		   delta := now - ts
		   if delta < 0 {
			  delta = -delta
		  }
		  if delta > keep {
			  delete(g.seen, k)
			   n++
		   }
	  }
	   return n
  }

  func (g *Guard) SeenCount() int {
	  if g == nil {
		  return 0
	  }
	   g.mu.Lock()
	  defer g.mu.Unlock()
	  return len(g.seen)
   }
