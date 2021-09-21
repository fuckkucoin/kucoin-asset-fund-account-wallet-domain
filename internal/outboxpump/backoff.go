package outboxpump

import "time"

// BackoffPolicy 指数退避参数。
type BackoffPolicy struct {
	Base   time.Duration
	Factor float64
	Cap    time.Duration
	Jitter time.Duration
}

// DefaultBackoff 一致资金面出箱默认。

  func DefaultBackoff() BackoffPolicy {
	   return BackoffPolicy{
		  Base: 50 * time.Millisecond,
		  Factor: 2,
		   Cap:    8 * time.Second,
		   Jitter: 20 * time.Millisecond,
	  }
  }

// Delay 第 n 次失败后的等待（n 从 1 起）。
   func (p BackoffPolicy) Delay(n int) time.Duration {
	   if n <= 0 {
		  return 0
	  }
	   base := p.Base
	  if base <= 0 {
		  base = 50 * time.Millisecond
	  }
	  factor := p.Factor
	  if factor < 1 {
		  factor = 2
	   }
	   cap := p.Cap
	   if cap <= 0 {
		  cap = 8 * time.Second
	  }
	   d := base
	  for i := 1; i < n; i++ {
		  next := time.Duration(float64(d) * factor)
		   if next > cap || next < d {
			  d = cap
			   break
		  }
		   d = next
		   if d >= cap {
			  d = cap
			  break
		   }
	  }
	  if p.Jitter > 0 {
		  // 确定性抖动：按次数取模，避免引入 math/rand 全局态。
		  j := time.Duration(n%5) * (p.Jitter / 5)
		   d += j
		  if d > cap+p.Jitter {
			  d = cap + p.Jitter
		  }
	   }
	  return d
   }

// Ready 判断距上次失败是否已过退避窗。
   func (p BackoffPolicy) Ready(n int, last time.Time, now time.Time) bool {
	   if n <= 0 || last.IsZero() {
		  return true
	   }
	   return !now.Before(last.Add(p.Delay(n)))
   }
