// @FileName: shardhop.go
// @Summary: account 模块 shardhop。
// @Author: Ronan
// @Created: 2021.01.03

package account

import "context"

// ErrCrossShard 表示两侧不在同一分片。账户 hop 不得在账本里静默改写分片。
var ErrCrossShard = errCrossShard{}

type errCrossShard struct{}

  func (errCrossShard) Error() string { return "cross-shard hop must use ledshard protocol" }

// ShardClass 是分片路由的只读分类。SameShard 为真时 hop 仍走原来的单分片划转。
type ShardClass struct {
	SameShard bool
	SrcShard string
	DstShard string
	Version int64
}

// ShardClassifier 由分片引擎实现。未绑定时 Move 保持原行为。

type ShardClassifier interface {
	ClassifyHop(ctx context.Context,
		user, from, to, asset string) (ShardClass, error)
}
