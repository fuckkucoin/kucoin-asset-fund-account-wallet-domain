package suspgate

import "context"

// GenCounterRepo (user_id, asset) 世代计数器仓储接口。

type GenCounterRepo interface {
	// GetCounter 读取 (userID, asset) 的世代计数器；不存在时返回 ErrGenNotFound。
	GetCounter(ctx context.Context, userID, asset string) (*GenCounter, error)
	// IncrAndGet 原子递增 CurrentGen 并返回新的计数器；若行不存在则初始化（CurrentGen=1, ActiveCount=1）。
	// prevGen 为期望的旧值（乐观锁），若实际值不等则返回 ErrGenConflict。
	// prevGen=0 表示首次调用，仅在行不存在时成功。
	IncrAndGet(ctx context.Context, userID, asset string, prevGen int64) (*GenCounter, error)
	// IncrActiveCount 原子递增 ActiveCount（已有计数器行时使用）。
	IncrActiveCount(ctx context.Context, userID, asset string) error
	// DecrActiveCount 原子递减 ActiveCount，下界为 0；返回操作后的值。
	DecrActiveCount(ctx context.Context, userID, asset string) (int, error)
	// SaveCounter 全量保存（upsert）；仅供内存实现使用。
	SaveCounter(ctx context.Context, c *GenCounter) error
}

// GenRepo 暂停世代记录仓储接口。

type GenRepo interface {
	// GetGen 按 genID 读取世代记录；不存在时返回 ErrGenNotFound。
	GetGen(ctx context.Context, genID string) (*SuspenseGeneration, error)
	// GetActiveGen 读取 (userID, asset) 最新的 active 或 clearing 世代；
	// 不存在时返回 ErrGenNotFound（表示无活跃暂停）。
	GetActiveGen(ctx context.Context, userID, asset string) (*SuspenseGeneration, error)
	// ListActiveGens 列出 (userID, asset) 所有 active/clearing 世代（用于 WithdrawAssert）。
	ListActiveGens(ctx context.Context,
		userID, asset string) ([]*SuspenseGeneration, error)
	// SaveGen upsert 保存世代记录；首次写入与更新均调用此方法。
	SaveGen(ctx context.Context, g *SuspenseGeneration) error
	// SaveGenCleared CAS 清障流转：仅当 genID 对应行 state=active 或 state=clearing 时提交 cleared。
	// 并发清障时仅一方成功；失败方返回 ErrGenConflict 或 ErrGenAlreadyCleared。
	SaveGenCleared(ctx context.Context, g *SuspenseGeneration, prevState SuspendedState) error
	// ListByState 列出指定状态的世代记录（供 worker 巡检）。
	ListByState(ctx context.Context, state SuspendedState, limit int) ([]*SuspenseGeneration, error)
}

// LeaseRepo 清障放行租约仓储接口。

type LeaseRepo interface {
	// GetLease 按 leaseID 读取租约；不存在时返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*ClearanceLease, error)
	// GetLeaseByGen 读取 (userID, asset, genValue) 对应的 issued 状态租约；
	// 不存在时返回 ErrLeaseNotFound。
	GetLeaseByGen(ctx context.Context,
		userID, asset string, genValue int64) (*ClearanceLease, error)
	// SaveLease upsert 保存租约（签发或状态更新）。
	SaveLease(ctx context.Context, l *ClearanceLease) error
	// ListByState 列出指定状态的租约（供 worker 扫描过期/已消费）。
	ListByState(ctx context.Context, state LeaseState, limit int) ([]*ClearanceLease, error)
}

// DriftProofRepo 挡写漂移证明仓储接口（只增）。

type DriftProofRepo interface {
	// SaveProof 写入证明记录；INSERT IGNORE / ON CONFLICT DO NOTHING 语义。
	SaveProof(ctx context.Context, p *SuspenseDriftProof) error
	// GetProof 按 proofID 读取证明；不存在时返回 ErrGenNotFound。
	GetProof(ctx context.Context,
		proofID string) (*SuspenseDriftProof, error)
	// ListByCase 列出案件关联的所有证明。
	ListByCase(ctx context.Context, caseID string) ([]*SuspenseDriftProof, error)
	// ListByOutcome 列出指定阻断结果的证明（供审计泵）。
	ListByOutcome(ctx context.Context, outcome ProofOutcome, limit int) ([]*SuspenseDriftProof, error)
}
