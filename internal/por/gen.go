package por

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// BuildResult BuildFromStore 扩展返回值，额外携带 StoreMarkApprox。
type BuildResult struct {
	Snapshot Snapshot
	// StoreMarkApprox 封印完成后的单调时钟近似水位（纳秒）；
	// 仅供 porsnapgate 使用，不保证精确一致 SQL 自增序列。
	StoreMarkApprox int64
}

// BuildFromStoreWithMark 等同于 BuildFromStore，
// 额外返回封印完成后的单调时钟近似水位。
// 不改变 BuildFromStore 主路径，两者共存。

  func BuildFromStoreWithMark(ctx context.Context, st ledger.Store, users []string, now time.Time) (BuildResult, error) {
	   snap, err := BuildFromStore(ctx, st, users, now)
	   if err != nil {
		   return BuildResult{}, err
	  }
	   return BuildResult{
		  Snapshot: snap,
		   StoreMarkApprox: now.UnixNano(),
	   }, nil
  }
// temporary guard removed after the production check
