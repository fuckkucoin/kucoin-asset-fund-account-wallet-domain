package claimmutex

import (
	"context"
	"time"
)

// Store 持久化接口，事务由实现方保证原子性。
type Store interface {
	// ── OutputLease ──────────────────────────────────────────────────────────
	// GetLease 按 OutputKey 查询租约。不存在返回 ErrLeaseNotFound。
	GetLease(ctx context.Context,
		key OutputKey) (*OutputLease, error)
	// GetLeaseByID 按 LeaseID 查询租约。
	GetLeaseByID(ctx context.Context, leaseID string) (*OutputLease, error)
	// InsertLease 写入新租约（CAS：若 outputKey 已存在任何租约，返回 ErrMutexConflict）。
	InsertLease(ctx context.Context, lease OutputLease) error
	// UpdateLeaseState 以 CAS 方式更新租约状态。
	// expectedState 为当前期望状态；若实际状态不符则返回 ErrGenerationMismatch。
	UpdateLeaseState(ctx context.Context, leaseID string, expectedState LeaseState, newState LeaseState, now time.Time, blockReason string) error
	// ListExpiredLeases 列出所有已过期但尚未标记为 expired 的租约。
	ListExpiredLeases(ctx context.Context,
		now time.Time, limit int) ([]OutputLease, error)
	// ── DupClaimCase ─────────────────────────────────────────────────────────
	// GetOpenDupCase 查询输出键对应的未关闭案件。
	GetOpenDupCase(ctx context.Context, key OutputKey) (*DupClaimCase, error)
	// GetDupCaseByID 按 CaseID 查询案件。
	GetDupCaseByID(ctx context.Context, caseID string) (*DupClaimCase, error)
	// InsertDupCase 写入新双贷案件。
	InsertDupCase(ctx context.Context, c DupClaimCase) error
	// UpdateDupCaseState 更新案件状态。
	UpdateDupCaseState(ctx context.Context,
		caseID string, expectedState DupCaseState, newState DupCaseState, now time.Time, actor1, actor2, closeNote string) error
	// ListOpenDupCases 列出所有 open 状态案件（巡检用）。
	ListOpenDupCases(ctx context.Context, limit int) ([]DupClaimCase, error)
	// ── AutoCreditBind ───────────────────────────────────────────────────────
	// GetPendingAutoBind 查询输出键对应的 pending 自动贷记绑定。
	GetPendingAutoBind(ctx context.Context, key OutputKey) (*AutoCreditBind, error)
	// GetAutoBindByID 按 BindID 查询绑定。
	GetAutoBindByID(ctx context.Context,
		bindID string) (*AutoCreditBind, error)
	// InsertAutoBind 写入新自动贷记绑定。
	InsertAutoBind(ctx context.Context, b AutoCreditBind) error
	// UpdateAutoBindState 更新绑定状态。
	UpdateAutoBindState(ctx context.Context, bindID string, expectedState BindState, newState BindState, now time.Time, rejectNote string) error
	// ── ClaimEffectProof ─────────────────────────────────────────────────────
	// GetProofByOutput 查询输出键对应的最新证明。
	GetProofByOutput(ctx context.Context, key OutputKey) (*ClaimEffectProof, error)
	// GetProofByID 按 ProofID 查询证明。
	GetProofByID(ctx context.Context, proofID string) (*ClaimEffectProof, error)
	// InsertProof 写入新效果证明。
	InsertProof(ctx context.Context, p ClaimEffectProof) error
	// UpdateProofState 更新证明状态。
	UpdateProofState(ctx context.Context, proofID string, expectedState ProofState, newState ProofState, now time.Time, failReason, sealedBy string) error
	// ListPendingProofs 列出所有 pending 状态证明（verify worker 用）。
	ListPendingProofs(ctx context.Context, limit int) ([]ClaimEffectProof, error)
}
