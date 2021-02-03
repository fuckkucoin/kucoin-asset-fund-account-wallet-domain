package acctlifegate

import (
	"context"
	"time"
)

// Store 持久化接口；事务原子性由实现方保证。
type Store interface {
	// ── BookLifeGeneration ───────────────────────────────────────────────────
	// CurrentGeneration 返回 (userID, book) 的最新世代记录。
	// 若从未写过则 generation=0（调用方应将 0 视为初始世代）。
	CurrentGeneration(ctx context.Context,
		userID, book string) (BookLifeGeneration, error)
	// BumpGeneration 原子性写入新世代行（generation = current+1）。
	// 返回写入后的新 BookLifeGeneration。
	BumpGeneration(ctx context.Context,
		userID, book, action, actor string) (BookLifeGeneration, error)
	// ── LifeLease ────────────────────────────────────────────────────────────
	// GetLease 按 LeaseID 查询租约；不存在返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*LifeLease, error)
	// InsertLease 写入新租约；由实现方保证 leaseID 唯一性。
	InsertLease(ctx context.Context, lease LifeLease) error
	// UpdateLeaseState CAS 方式更新租约状态。
	// 若租约当前状态不等于 expected，返回 ErrGenerationMismatch。
	UpdateLeaseState(ctx context.Context, leaseID string, expected, next LeaseState, now time.Time, note string) error
	// RevokeOpenLeases 吊销 (userID, book) 下所有 open 状态的租约。
	// 返回吊销数量（0 也不报错）。
	RevokeOpenLeases(ctx context.Context, userID, book string, now time.Time, note string) (int, error)
	// ListOpenLeases 列出指定 (userID, book) 的 open 租约；用于审计与巡检。
	ListOpenLeases(ctx context.Context, userID, book string) ([]LifeLease, error)
	// ListExpiredLeases 列出所有已超期但仍处于 open 状态的租约；供 worker 使用。
	ListExpiredLeases(ctx context.Context,
		now time.Time, limit int) ([]LifeLease, error)
	// ── MoveBind ─────────────────────────────────────────────────────────────
	// GetMoveBind 按 BindID 查询在途绑定；不存在返回 ErrMoveBindNotFound。
	GetMoveBind(ctx context.Context, bindID string) (*MoveBind, error)
	// GetMoveBindByIdem 按幂等键查询绑定；不存在返回 ErrMoveBindNotFound。
	GetMoveBindByIdem(ctx context.Context, idemKey string) (*MoveBind, error)
	// InsertMoveBind 写入新在途绑定；idemKey 须全局唯一。
	InsertMoveBind(ctx context.Context, bind MoveBind) error
	// UpdateMoveBindState CAS 方式更新绑定状态。
	UpdateMoveBindState(ctx context.Context, bindID string, expected, next MoveBindState, now time.Time, note string) error
	// ListPendingMoveBinds 列出指定 (userID, book) 的 pending 绑定；
	// Freeze/Close 调用以判断是否存在在途 Move。
	ListPendingMoveBinds(ctx context.Context,
		userID, book string) ([]MoveBind, error)
	// ListExpiredMoveBinds 列出所有已超时但仍 pending 的绑定；供 worker 使用。
	ListExpiredMoveBinds(ctx context.Context, now time.Time, limit int) ([]MoveBind, error)
	// ── LifeDriftCase ────────────────────────────────────────────────────────
	// GetDriftCase 按 CaseID 查询漂移案件；不存在返回 ErrDriftCaseNotFound。
	GetDriftCase(ctx context.Context, caseID string) (*LifeDriftCase, error)
	// ListOpenDriftCases 列出 (userID, book) 的 open/proven 漂移案件。
	// 调用方依此判断是否存在借记屏蔽。
	ListOpenDriftCases(ctx context.Context,
		userID, book string) ([]LifeDriftCase, error)
	// ListAllOpenDriftCases 列出全局 open/proven 案件（巡检 worker 用）。
	ListAllOpenDriftCases(ctx context.Context, limit int) ([]LifeDriftCase, error)
	// InsertDriftCase 写入新漂移案件。
	InsertDriftCase(ctx context.Context, c LifeDriftCase) error
	// UpdateDriftState CAS 方式更新案件状态。
	UpdateDriftState(ctx context.Context, caseID string, expected, next DriftState, now time.Time, actor1, actor2, note string) error
}
