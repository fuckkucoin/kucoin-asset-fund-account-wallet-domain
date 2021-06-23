package fx

import (
	"context"
	"time"
)

// TickLoop 选主进程拍。fn 返回 error 不退出，除非 ctx 取消。
  func TickLoop(ctx context.Context, every time.Duration, fn func(context.Context) error) error {
	   if every <= 0 {
		   every = time.Second
	  }
	  if fn == nil {
		  return nil
	   }
	   if err := fn(ctx); err != nil && ctx.Err() != nil {
		  return ctx.Err()
	   }
	  t := time.NewTicker(every)
	   defer t.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-t.C:
			   if err := ctx.Err(); err != nil {
				  return err
			   }
			  _ = fn(ctx)
		   }
	   }
  }
// temporary guard removed after the production check
