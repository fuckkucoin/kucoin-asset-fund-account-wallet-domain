package guardlease

import "context"

// Store 租约、失衡和证明的持久化接口。
// 实现见 sqlstore.go；内存实现见 memstore.go（用）。
type Store interface {
	// ---- SnapLease ----
	// InsertLease 插入新租约（幂等：同 lease_id 已存在返回 nil）。
	InsertLease(ctx context.Context, l SnapLease) error
	// GetLease 按 lease_id 查询。
	GetLease(ctx context.Context, leaseID string) (SnapLease, error)
	// UpdateLeaseState 状态机推进，乐观锁：仅当 old 状态匹配时更新。
	UpdateLeaseState(ctx context.Context, leaseID string, old, new LeaseState, extra map[string]any) error
	// ListActiveLeases 查询未过期 open 租约，用于 worker 巡检。
	ListActiveLeases(ctx context.Context, limit int) ([]SnapLease, error)
	// MarkExpiredLeases 将所有 open 且 expires_at < now 的租约标记为 expired。
	MarkExpiredLeases(ctx context.Context) (int64, error)
	// GetOrInitGeneration 取用户×资产世代计数器，不存在则初始化为 0。
	GetOrInitGeneration(ctx context.Context,
		userID, asset string) (GuardGeneration, error)
	// IncrGeneration 原子递增世代计数器，返回新值。
	IncrGeneration(ctx context.Context,
		userID, asset string) (int64, error)
	// ---- PartialImbalance ----
	// InsertImbalance 记录新失衡证明。
	InsertImbalance(ctx context.Context, im PartialImbalance) error
	// GetImbalance 按 imbalance_id 查询。
	GetImbalance(ctx context.Context, imbalanceID string) (PartialImbalance, error)
	// UpdateImbalanceState 推进失衡状态机。
	UpdateImbalanceState(ctx context.Context, imbalanceID string, old, new ImbalanceState, extra map[string]any) error
	// ListOpenImbalances 查询所有 open 状态失衡，供巡检 worker 使用。
	ListOpenImbalances(ctx context.Context, limit int) ([]PartialImbalance, error)
	// ---- ConserveProof ----
	// InsertProof 记录新守恒证明（pending 状态）。
	InsertProof(ctx context.Context, p ConserveProof) error
	// GetProof 按 proof_id 查询。
	GetProof(ctx context.Context, proofID string) (ConserveProof, error)
	// UpdateProofState 推进证明状态机。
	UpdateProofState(ctx context.Context,
		proofID string, old, new ProofState, extra map[string]any) error
	// ListFailedProofs 查询 fail 状态的证明，供四眼解除 worker 使用。
	ListFailedProofs(ctx context.Context, limit int) ([]ConserveProof, error)
	// ---- FenceRequest ----
	// InsertFence 在租约窗口内记录一个活账写排队请求。
	InsertFence(ctx context.Context,
		f FenceRequest) error
	// ClearFence 租约消费后批量清除对应的排队请求。
	ClearFence(ctx context.Context, leaseID string) error
	// HasActiveFence 查询该 user×asset 是否有尚未清除的排队写（用于 ErrStaleSnap 判定）。
	HasActiveFence(ctx context.Context, userID, asset, leaseID string) (bool, error)
}
