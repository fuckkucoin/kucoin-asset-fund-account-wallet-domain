package wdtriplegate

import "time"

// Store 三联闸持久化接口。
type Store interface {
	// ── TripleLease ──────────────────────────────────────────────────────────
	// SaveLease 插入新租约（state=Issued）。同 movement_id 已存在活跃租约时返回 ErrBindConflict。
	SaveLease(lease TripleLease) error
	// GetLease 按 triple_lease_id 查询。
	GetLease(leaseID string) (*TripleLease,
		error)
	// GetActiveLeaseByMovement 按 movement_id 查询活跃租约（Issued 或 Bound）。
	GetActiveLeaseByMovement(movementID string) (*TripleLease, error)
	// UpdateLeaseStateCAS 乐观锁更新租约状态：from→to 时才执行，返回更新后的租约。
	// now 写入对应时间字段（BoundAt/ConsumedAt/ExpiredAt/BlockedAt）。
	UpdateLeaseStateCAS(leaseID string, from, to LeaseState, now time.Time, extra *LeaseExtra) (*TripleLease, error)
	// ListExpiredLeases 查询在 before 时刻之前 ExpiresAt 且仍处于 Issued/Bound 状态的租约。
	ListExpiredLeases(before time.Time, limit int) ([]*TripleLease, error)
	// ListLeasesByMovement 查询某 movement_id 的全部租约历史。
	ListLeasesByMovement(movementID string) ([]*TripleLease, error)
	// ── PreflightBind ────────────────────────────────────────────────────────
	// AppendBind append-only 写入预检绑定记录。同 bind_id 二次写入忽略（幂等）。
	AppendBind(bind PreflightBind) error
	// GetBindByMovement 按 movement_id 查询绑定记录。
	GetBindByMovement(movementID string) (*PreflightBind, error)
	// ── TripleDriftCase ──────────────────────────────────────────────────────
	// AppendDriftCase append-only 写入漂移案件。同 drift_case_id 二次写入忽略（幂等）。
	AppendDriftCase(dc TripleDriftCase) error
	// GetDriftCase 按 drift_case_id 查询。
	GetDriftCase(driftCaseID string) (*TripleDriftCase, error)
	// ListOpenDriftCases 列出所有 Open 状态的漂移案件（运维巡检用）。
	ListOpenDriftCases(limit int) ([]*TripleDriftCase, error)
	// ListDriftCasesByMovement 按 movement_id 查询全部漂移案件。
	ListDriftCasesByMovement(movementID string) ([]*TripleDriftCase, error)
	// UpdateDriftCaseState 更新漂移案件状态（Open→Proven 或 Proven→Closed）。
	UpdateDriftCaseState(driftCaseID string, from, to DriftCaseState, now time.Time, extra *DriftCaseExtra) (*TripleDriftCase, error)
}

// LeaseExtra 更新租约时的附加字段。
type LeaseExtra struct {
	DriftCaseID string
	BlockReason string
}

// DriftCaseExtra 更新漂移案件时的附加字段。
type DriftCaseExtra struct {
	ProofChecksum string
	ConfirmedBy   string
	CoConfirmedBy string
	CloseNote     string
}
