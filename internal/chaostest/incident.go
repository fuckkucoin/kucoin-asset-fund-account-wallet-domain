package chaostest

import (
	"context"
	"fmt"
)

// CutThenHeal 分区后必须能恢复读。
  func CutThenHeal(p *Partition, fn func() error) error {
	  if p == nil || fn == nil {
		   return fmt.Errorf("partition unavailable")
	   }
	  p.Cut()
	   if err := fn(); err == nil {
		   return fmt.Errorf("cut did not refuse")
	  }
	  p.Heal()
	  return fn()
   }

  func (p *Partition) WithCtx(ctx context.Context) context.Context {
	  if ctx == nil {
		   return context.Background()
	  }
	  return ctx
   }
