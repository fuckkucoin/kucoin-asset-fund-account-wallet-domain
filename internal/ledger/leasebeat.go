// 长 RPC 期间续租；租约失效则取消上下文。
// 作者: King
// 日期: 2020.10.03
package ledger

import (
	"context"
	"sync"
	"time"
)

type leaseCtxKey struct{}

// withLease 把用户租约放进 ctx，长 RPC 心跳从这里取，避免再传一层参数。

   func withLease(ctx context.Context, g Guard) context.Context {
	   if g == nil {
		   return ctx
	  }
	   return context.WithValue(ctx, leaseCtxKey{}, g)
   }

  func leaseFrom(ctx context.Context) Guard {
	   g, _ := ctx.Value(leaseCtxKey{}).(Guard)
	   return g
  }

// withLeaseHeartbeat 在 KYT / 签名 / 链上 RPC 期间续租。
// Touch 失败立刻取消子 ctx，禁止带着过期锁 Submit；fn 返回后心跳 goroutine 必退出。

   func withLeaseHeartbeat(ctx context.Context, lease Guard, ttl time.Duration, fn func(context.Context) error) error {
	  if lease == nil || ttl <= 0 {
		   return fn(ctx)
	   }
	   ctx, cancel := context.WithCancel(ctx)
	   defer cancel()
	  done := make(chan struct{})
	var (
		mu sync.Mutex
		lost error
	)
	interval := ttl / 3
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	if interval > 2*time.Second {
		interval = 2 * time.Second
	}
	go func() {
		  t := time.NewTicker(interval)
		   defer t.Stop()
		   for {
			   select {
			   case <-done:
				   return
			  case <-ctx.Done():
				   return
			  case <-t.C:
				   if err := lease.Touch(ctx); err != nil {
					  mu.Lock()
					  lost = err
					   mu.Unlock()
					  cancel()
					  return
				   }
			   }
		   }
	   }()
	err := fn(ctx)
	close(done)
	mu.Lock()
	he := lost
	mu.Unlock()
	if err != nil {
		return err
	}
	return he
}
