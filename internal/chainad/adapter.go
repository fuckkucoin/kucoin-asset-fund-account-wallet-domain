package chainad

import (
	"context"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

// Observer 充值观测。金额只认链上结果，调用方不得另造。
type Observer interface {
	Family() Family
	ObserveInbound(ctx context.Context, hash, to string, amount decimal.Decimal) (Inbound, error)
	Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error)
}

// Submitter 出金广播。同一 nonce 必须回放原哈希，禁止双播。
type Submitter interface {
	Family() Family
	SubmitWithdraw(ctx context.Context, req Outbound) (Receipt, error)
	Receipt(ctx context.Context, hash string) (Receipt, error)
}

// Adapter 一族链的观察+广播。账本/充提只依赖该面，不绑具体节点 SDK。
type Adapter interface {
	Observer
	Submitter
	NativeAsset() string
	ConfirmNeed() int
	Head() (height int64, blockHash string)
	Mine(hashes ...string) int64
	Reorg(drop int) []string
	PutInbound(in Inbound) error
}
