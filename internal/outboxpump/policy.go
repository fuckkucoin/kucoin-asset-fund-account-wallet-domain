package outboxpump

import "time"

// SetBackoffPolicy 替换固定退避为策略计算。

  func (r *Retry) SetBackoffPolicy(p BackoffPolicy) {
	   if r == nil {
		  return
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  r.policy = &p
	  if p.Base > 0 {
		   r.backoff = p.Base
	   }
  }

// MaxAttempts 当前毒丸阈值。

  func (r *Retry) MaxAttempts() int {
	  if r == nil || r.max <= 0 {
		  return 5
	   }
	   return r.max
   }

  func (r *Retry) waitOK(st *Attempt, now time.Time) bool {
	  if st == nil || st.N <= 0 {
		   return true
	   }
	   if r.policy != nil {
		   return r.policy.Ready(st.N, st.Last, now)
	   }
	   return now.Sub(st.Last) >= r.backoff
  }
