package fundintgate

import (
	"context"
	"time"
)

// Store 定义 fundintgate 持久化操作。
type Store interface {
	SaveLease(ctx context.Context, lease IntGateLease) error
	GetLease(ctx context.Context,
		leaseID string) (IntGateLease, error)
	GetLeaseByIdempotencyKey(ctx context.Context, key string) (IntGateLease, bool, error)
	UpdateLease(ctx context.Context, lease IntGateLease) error
	ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]IntGateLease, error)
	ListExpiredGated(ctx context.Context,
		now time.Time, limit int) ([]IntGateLease, error)
	ListUnconsumed(ctx context.Context, limit int) ([]IntGateLease, error)
	ListRevocableByGen(ctx context.Context, maxGen int64, limit int) ([]IntGateLease, error)
	SaveGen(ctx context.Context, gen GateGeneration) error
	GetGen(ctx context.Context, scope string) (GateGeneration, error)
	SaveProof(ctx context.Context, proof IntGateDriftProof) error
	GetProof(ctx context.Context, proofID string) (IntGateDriftProof, error)
	ListProofs(ctx context.Context, filter ExportProofFilter) ([]IntGateDriftProof, error)
	MarkProofsExported(ctx context.Context, ids []string, at time.Time) error
	SaveAudit(ctx context.Context, row IntGateAudit) error
	ListAuditByLease(ctx context.Context,
		leaseID string, limit int) ([]IntGateAudit, error)
}

// Sender 账本发送挂钩。本包不改写 SpotSend 公式，只调用该接口。
type Sender interface {
	SpotSend(ctx context.Context, from, to, asset, amount, idem string) (transferID string, err error)
}

// Resolver 对手解析挂钩。只读消费 userdir 解析结果。
type Resolver interface {
	Resolve(ctx context.Context, toUID, toEmail, toPhone string) (ResolveResult, error)
}

// GateEval 闸判定挂钩。只读调用既有 GateFunc 语义。

type GateEval interface {
	Allow(ctx context.Context, userID string) error
}
