package invleasegate

import (
	"context"
	"time"
)

// Store invleasegate 持久化接口。
type Store interface {
	// ── InvPrecheckLease ─────────────────────────────────────────────────────
	// SaveLease 新建租约（insert-only）。
	SaveLease(ctx context.Context, l *InvPrecheckLease) error
	// GetLease 按 lease_id 查询；未找到返回 ErrLeaseNotFound。
	GetLease(ctx context.Context,
		id string) (*InvPrecheckLease, error)
	// UpdateLeaseState CAS 更新租约状态（from→to）并附加 patch 字段；
	// 状态不符合预期（即当前状态 ≠ from）时返回 ErrInvalidTransition。
	UpdateLeaseState(ctx context.Context,
		id string, from, to LeaseState, patch *InvPrecheckLease) error
	// ListActiveLeasesForAsset 查询指定资产 issued/bound 状态的租约（按 issued_at 降序）。
	ListActiveLeasesForAsset(ctx context.Context, asset string, limit int) ([]*InvPrecheckLease, error)
	// ListExpiredLeases 查询超期（expires_at <= cutoff）且非终态的租约。
	ListExpiredLeases(ctx context.Context, cutoff time.Time, limit int) ([]*InvPrecheckLease, error)
	// ListLeasesByAsset 查询资产下的所有租约（分页，按 issued_at 降序）。
	ListLeasesByAsset(ctx context.Context, asset string, limit int) ([]*InvPrecheckLease, error)
	// ── InvGateDriftProof ────────────────────────────────────────────────────
	// SaveProof 新建漂移证明；同 lease_id 二次调用返回 ErrAlreadyProven。
	SaveProof(ctx context.Context, p *InvGateDriftProof) error
	// GetProof 按 proof_id 查询；未找到返回 ErrProofNotFound。
	GetProof(ctx context.Context,
		id string) (*InvGateDriftProof, error)
	// GetProofByLease 按 lease_id 查询；未找到返回 ErrProofNotFound。
	GetProofByLease(ctx context.Context, leaseID string) (*InvGateDriftProof, error)
	// UpdateProofState 更新证明状态（open→closed），附加 closed_at。
	UpdateProofState(ctx context.Context, id string, from, to ProofState, patch *InvGateDriftProof) error
	// ListOpenProofs 查询 open 状态的漂移证明（供审计巡检）。
	ListOpenProofs(ctx context.Context, limit int) ([]*InvGateDriftProof, error)
}
