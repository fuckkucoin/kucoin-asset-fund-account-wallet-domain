package wdpartial

import "context"

// HoldStore Hold 持久化接口。

type HoldStore interface {
	// Create 新建 Hold 记录（幂等：同 ID 重复写返回已有记录，不报错）。
	Create(ctx context.Context,
		h *HoldRecord) error
	// Get 按 ID 查询。
	Get(ctx context.Context, id string) (*HoldRecord, error)
	// GetByPlan 查询计划下的全部 Hold。
	GetByPlan(ctx context.Context, planID string) ([]*HoldRecord, error)
	// Update 更新 Hold 状态（乐观锁：updated_at 不匹配则返回错误）。
	Update(ctx context.Context, h *HoldRecord) error
	// ListOpenByPlan 查询计划下状态为 open 的 Hold（用于释放路径）。
	ListOpenByPlan(ctx context.Context, planID string) ([]*HoldRecord, error)
}

// FillStore Fill 持久化接口。
type FillStore interface {
	// Create 新建 Fill 记录。
	Create(ctx context.Context, f *FillRecord) error
	// Get 按 ID 查询。
	Get(ctx context.Context, id string) (*FillRecord, error)
	// GetByPlan 查询计划下全部 Fill。
	GetByPlan(ctx context.Context, planID string) ([]*FillRecord, error)
	// Update 更新 Fill 状态。
	Update(ctx context.Context, f *FillRecord) error
	// ListConfirmedByPlan 查询计划下已确认的 Fill（用于守恒汇总）。
	ListConfirmedByPlan(ctx context.Context, planID string) ([]*FillRecord, error)
}

// LeaseStore RemReleaseLease 持久化接口。
type LeaseStore interface {
	// Create 新建租约。
	Create(ctx context.Context,
		l *RemReleaseLease) error
	// Get 按 ID 查询。
	Get(ctx context.Context, id string) (*RemReleaseLease, error)
	// GetOpenByPlan 查询计划下处于 open 状态的租约（须加行锁防并发）。
	// 不存在时返回 ErrLeaseNotFound。
	GetOpenByPlan(ctx context.Context, planID string) (*RemReleaseLease, error)
	// Update 更新租约状态。
	Update(ctx context.Context, l *RemReleaseLease) error
	// ListExpired 查询已过期但未 revoked 的租约（Worker 吊销用）。
	ListExpired(ctx context.Context,
		limit int) ([]*RemReleaseLease, error)
}

// ConserveCaseStore ConserveCase 持久化接口。
type ConserveCaseStore interface {
	// Create 新建守恒案件（conserve_snapshot 一经写入不可修改）。
	Create(ctx context.Context,
		c *ConserveCase) error
	// Get 按 ID 查询。
	Get(ctx context.Context, id string) (*ConserveCase, error)
	// GetOpenByPlan 查询计划下处于 open 状态的守恒案件。
	// 存在时阻断 ConsumeHold；不存在时返回 ErrConserveCaseNotFound。
	GetOpenByPlan(ctx context.Context, planID string) (*ConserveCase, error)
	// Update 更新案件状态（conserve_snapshot 字段在实现层禁止更新）。
	Update(ctx context.Context, c *ConserveCase) error
	// ListOpen 查询全部 open 案件（Worker 巡检用）。
	ListOpen(ctx context.Context, limit int) ([]*ConserveCase, error)
}

// GuardGenStore BroadcastGuardGen 持久化接口。
type GuardGenStore interface {
	// Create 新建守卫世代。
	Create(ctx context.Context,
		g *BroadcastGuardGen) error
	// Get 按 ID 查询。
	Get(ctx context.Context, id string) (*BroadcastGuardGen, error)
	// GetActiveByPlan 查询计划下活跃守卫世代（用于 skip 腿释放前校验）。
	GetActiveByPlan(ctx context.Context, planID string) (*BroadcastGuardGen, error)
	// Update 更新守卫世代状态。
	Update(ctx context.Context, g *BroadcastGuardGen) error
	// ListActive 查询全部 active 世代（Worker 巡检用）。
	ListActive(ctx context.Context,
		limit int) ([]*BroadcastGuardGen, error)
}
