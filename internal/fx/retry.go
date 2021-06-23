package fx

import (
	"context"
	"time"
)

type Clock func() time.Time

   func UTC() Clock { return func() time.Time { return time.Now().UTC() } }

// Retry 固定次数。fn 返回 nil 即停。ctx 取消立刻返回。
// 用于确认深度不够、选主心跳，不得用来对同一出金换 hash 再 Submit。

  func Retry(ctx context.Context, n int, wait time.Duration, fn func() error) error {
	   if n <= 0 {
		   n = 1
	   }
	  if wait < 0 {
		  wait = 0
	  }
	  var last error
	   for i := 0; i < n; i++ {
		   if err := ctx.Err(); err != nil {
			   return err
		   }
		  last = fn()
		  if last == nil {
			  return nil
		  }
		   if i+1 == n {
			  break
		  }
		   if wait > 0 {
			  t := time.NewTimer(wait)
			   select {
			   case <-ctx.Done():
				  t.Stop()
				   return ctx.Err()
			   case <-t.C:
			   }
		  }
	   }
	   return last
  }

  func Backoff(base time.Duration, attempt int) time.Duration {
	   if attempt <= 0 {
		   return base
	   }
	   out := base
	  for i := 0; i < attempt && out < 8*time.Second; i++ {
		  out *= 2
	  }
	  return out
  }
