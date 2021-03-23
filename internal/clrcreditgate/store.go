package clrcreditgate

import (
	"context"
	"time"
)

// Store clrcreditgate 持久化接口。
type Store interface {
	// ── ReleaseCreditLease ───────────────────────────────────────────────────
	// SaveLease 新建释放贷记租约（insert-only，幂等：同 LeaseID 二次写忽略）。
	// 若同 ItemID 已有 State=issued 的 Lease，返回 ErrLeaseAlreadyIssued。
	SaveLease(ctx context.Context, l *ReleaseCreditLease) error
	// GetLease 按 LeaseID 查询；未找到返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*ReleaseCreditLease, error)
	// GetLeaseByItem 按 ItemID 查询最新（issued 先）Lease；未找到返回 ErrLeaseNotFound。
	GetLeaseByItem(ctx context.Context, itemID string) (*ReleaseCreditLease, error)
	// ConsumeLease CAS 消费：State=issued 且 Fence 匹配时原子迁移至 consumed。
	// State 不为 issued 时返回 ErrLeaseConsumed / ErrLeaseExpired / ErrLeaseVoid；
	// Fence 不匹配时返回 ErrLeaseFenceStale。
	ConsumeLease(ctx context.Context, leaseID string, fence Fence, proofID string, at time.Time) error
	// MarkLeaseExpired 将 State=issued 的 Lease 标记为 expired；State 不为 issued 返回 ErrInvalidTransition。
	MarkLeaseExpired(ctx context.Context, leaseID string, at time.Time) error
	// MarkLeaseVoid 将 State=issued 的 Lease 标记为 void，并记录原因。
	// State 不为 issued 返回 ErrInvalidTransition。
	MarkLeaseVoid(ctx context.Context, leaseID string, reason string, at time.Time) error
	// ListLeases 按过滤条件查询 Lease 列表（按 issued_at 降序）。
	ListLeases(ctx context.Context, f LeaseListFilter) ([]*ReleaseCreditLease, error)
	// CountUnconsumedLeases 返回指定 BatchID 中 State=issued 且未过期的 Lease 数量。
	// 供 CloseBatchGuard 快速判断是否阻断关批。
	CountUnconsumedLeases(ctx context.Context, batchID string, now time.Time) (int, error)
	// ── ClearingCreditProof ──────────────────────────────────────────────────
	// SaveProof 写入交收证明（insert-only，幂等：同 ProofID 二次写忽略）。
	// 若同 ItemID + Kind=credit_ok 已存在证明，返回 ErrAlreadyProven（防双贷）。
	SaveProof(ctx context.Context, p *ClearingCreditProof) error
	// GetProof 按 ProofID 查询；未找到返回 ErrProofNotFound。
	GetProof(ctx context.Context, proofID string) (*ClearingCreditProof, error)
	// GetOKProofByItem 按 ItemID 查询 Kind=credit_ok 的证明；未找到返回 ErrProofNotFound。
	GetOKProofByItem(ctx context.Context, itemID string) (*ClearingCreditProof, error)
	// ListProofs 按过滤条件查询 Proof 列表（按 at 降序）。
	ListProofs(ctx context.Context, f ProofListFilter) ([]*ClearingCreditProof, error)
	// ── BatchDrainState ──────────────────────────────────────────────────────
	// GetOrCreateBatchDrain 查询批次排空状态；不存在时自动创建 Status=pending 记录。
	GetOrCreateBatchDrain(ctx context.Context, batchID string, now time.Time) (*BatchDrainState, error)
	// UpdateBatchDrainStatus CAS 更新排空状态（from→to）；from 不匹配时返回 ErrInvalidTransition。
	UpdateBatchDrainStatus(ctx context.Context, batchID string, from, to DrainStatus, patch *BatchDrainState) error
	// GetBatchDrain 按 BatchID 查询排空状态；未找到返回 ErrBatchNotFound。
	GetBatchDrain(ctx context.Context, batchID string) (*BatchDrainState, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// Fence（进程内别名，避免循环导入）
// ──────────────────────────────────────────────────────────────────────────────
// Fence 行级围栏（来自 persist.Fence，此处用进程内结构减少外部依赖侵入）。
type Fence struct {
	Epoch int64 `json:"epoch"`
	Token string `json:"token"`
}
