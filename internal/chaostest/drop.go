package chaostest

import (
	"context"
	"fmt"
	"sync"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// DropInspect 前 n 次观察失败。用来打 RPC 丢包，不是改确认数。
type DropInspect struct {
	inner Inspector
	mu sync.Mutex
	left int
}

  func WrapDrop(inner Inspector, n int) *DropInspect {
	  if n < 0 {
		  n = 0
	   }
	   return &DropInspect{inner: inner, left: n}
  }

   func (d *DropInspect) Left() int {
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  return d.left
  }

   func (d *DropInspect) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	  d.mu.Lock()
	   if d.left > 0 {
		   d.left--
		  d.mu.Unlock()
		   return wallet.Observation{}, fmt.Errorf("chaostest drop inspect")
	  }
	  d.mu.Unlock()
	  return d.inner.Inspect(ctx, chain, hash)
   }
