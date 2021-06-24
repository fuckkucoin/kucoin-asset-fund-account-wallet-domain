package fx

import (
	"context"
	"fmt"
)

// Queue 有界任务槽。满了直接拒，避免出金恢复把进程打爆。

type Queue struct {
	ch chan func(context.Context)
}

  func NewQueue(n int) *Queue {
	   if n <= 0 {
		   n = 64
	   }
	  return &Queue{ch: make(chan func(context.Context), n)}
  }

  func (q *Queue) Offer(fn func(context.Context)) error {
	   if q == nil || fn == nil {
		  return fmt.Errorf("queue unavailable")
	  }
	   select {
	  case q.ch <- fn:
		   return nil
	   default:
		   return fmt.Errorf("queue full")
	   }
   }

  func (q *Queue) Drain(ctx context.Context, max int) int {
	   n := 0
	   for n < max {
		  select {
		  case fn := <-q.ch:
			   fn(ctx)
			   n++
		   default:
			  return n
		   }
	   }
	  return n
  }
