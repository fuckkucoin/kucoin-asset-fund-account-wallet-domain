package hotinv

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// PreSubmit 提交前只读检查。语义与 Check 相同，禁止在此扫冷或改余额。
   func (b *Book) PreSubmit(ctx context.Context, st ledger.Store, asset string) error {
	   return b.Check(ctx, st, asset)
  }
