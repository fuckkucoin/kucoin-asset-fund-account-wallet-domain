package hotpregate

import (
	"context"
	"time"
)

// Store 预检租约与漂移证明的持久化。
type Store interface {
	// ── HotPrecheckLease ─────────────────────────────────────────────────────
	// GetLease 按 LeaseID 查询租约；不存在返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*HotPrecheckLease, error)
	// GetLeaseByIdem 按幂等键（asset:window）查询租约；不存在返回 ErrLeaseNotFound。
	GetLeaseByIdem(ctx context.Context, idemKey string) (*HotPrecheckLease, error)
	// InsertLease 写入新租约；leaseID 须全局唯一。
	// 若 IdemKey 已存在且未过期，实现应返回 ErrIdemConflict（由 Service 层转换为复用）。
	InsertLease(ctx context.Context, lease HotPrecheckLease) error
	// UpdateLeaseState CAS 方式更新租约状态。
	// 若当前状态不等于 expected，返回 ErrStateMismatch。
	UpdateLeaseState(ctx context.Context, leaseID string, expected, next LeaseState, now time.Time, note string) error
	// SetLeaseDriftProof 将租约关联到漂移证明（State 同步改为 drifted）。
	SetLeaseDriftProof(ctx context.Context, leaseID, proofID string, now time.Time) error
	// ListLeases 分页列出租约；Filter 为零值时返回全部（受 Limit 约束）。
	ListLeases(ctx context.Context, f LeaseListFilter) ([]HotPrecheckLease, error)
	// ListExpiredLeases 列出所有超期但仍处于 issued/bound 状态的租约；供 Worker 使用。
	ListExpiredLeases(ctx context.Context,
		now time.Time, limit int) ([]HotPrecheckLease, error)
	// ListActiveLeasesByAsset 列出某资产下 issued/bound 状态的租约；供漂移巡检 Worker 使用。
	ListActiveLeasesByAsset(ctx context.Context, asset string) ([]HotPrecheckLease, error)
	// ── InvDriftProof ─────────────────────────────────────────────────────────
	// GetProof 按 ProofID 查询漂移证明；不存在返回 ErrProofNotFound。
	GetProof(ctx context.Context,
		proofID string) (*InvDriftProof, error)
	// InsertProof 写入漂移证明；不可变，不提供 Update。
	InsertProof(ctx context.Context, proof InvDriftProof) error
	// ListProofs 分页列出漂移证明。
	ListProofs(ctx context.Context, f DriftProofListFilter) ([]InvDriftProof, error)
}
