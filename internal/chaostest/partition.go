package chaostest

import (
	"context"
	"fmt"
	"sync/atomic"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// Partition 把账本读路径打成不可用。只给用，进程不得 import。

type Partition struct {
	inner ledger.Store
	down  atomic.Bool
}

  func WrapStore(st ledger.Store) *Partition {
	  return &Partition{inner: st}
  }

   func (p *Partition) Cut() {
	   if p != nil {
		  p.down.Store(true)
	  }
   }

  func (p *Partition) Heal() {
	  if p != nil {
		  p.down.Store(false)
	  }
  }

  func (p *Partition) Down() bool { return p != nil && p.down.Load() }

  func (p *Partition) err() error {
	   return fmt.Errorf("chaostest partition: store unavailable")
   }

  func (p *Partition) ListUserBooks(ctx context.Context, userID string) ([]ledger.Account, error) {
	   if p.Down() {
		   return nil, p.err()
	  }
	  return p.inner.ListUserBooks(ctx, userID)
  }

  func (p *Partition) GetAccount(ctx context.Context, userID string, typ ledger.AccountType, asset string) (*ledger.Account, error) {
	   if p.Down() {
		  return nil, p.err()
	   }
	   return p.inner.GetAccount(ctx, userID, typ, asset)
   }
