package keyrotgate

import "context"

// Store keyrotgate 持久化接口。

type Store interface {
	// ── RotateLease ──────────────────────────────────────────────────────────
	// SaveLease 新建或更新轮换租约（upsert）。
	SaveLease(ctx context.Context, l *RotateLease) error
	// GetLease 按 rotate_lease_id 查询租约。
	GetLease(ctx context.Context,
		id string) (*RotateLease, error)
	// GetLeaseByTicket 按 ticket_id 查询租约（一个 Ticket 对应一个 Lease）。
	GetLeaseByTicket(ctx context.Context, ticketID string) (*RotateLease, error)
	// ListActiveLeasesBySlot 查询指定源槽的非终态租约列表。
	ListActiveLeasesBySlot(ctx context.Context, srcSlotID string) ([]*RotateLease, error)
	// UpdateLeaseState 用 CAS 更新租约状态（from→to）。
	// 若当前状态不等于 from，返回 ErrInvalidTransition。
	UpdateLeaseState(ctx context.Context, id string, from, to LeaseState, update *RotateLease) error
	// ListExpiredLeases 查询已超期（ExpiresAt < now）且状态在 Open/Approved/Executing 的租约。
	ListExpiredLeases(ctx context.Context, limit int) ([]*RotateLease, error)
	// ── SignBind ──────────────────────────────────────────────────────────────
	// SaveSignBind 新建签名绑定记录。
	SaveSignBind(ctx context.Context, b *SignBind) error
	// GetSignBind 按 sign_bind_id 查询。
	GetSignBind(ctx context.Context, id string) (*SignBind, error)
	// GetSignBindByOperation 按 operation_id 查询（防重复绑定）。
	GetSignBindByOperation(ctx context.Context, operationID string) (*SignBind, error)
	// ListActiveSignBinds 查询指定源槽的所有 Active 状态绑定。
	// 用于 ExecuteRotate 前检测在途签名互斥。
	ListActiveSignBinds(ctx context.Context,
		srcSlotID string) ([]*SignBind, error)
	// UpdateSignBindState CAS 更新签名绑定状态。
	UpdateSignBindState(ctx context.Context, id string, from, to SignBindState, update *SignBind) error
	// ListExpiredSignBinds 查询已超期（ExpiresAt < now）且 Active 的绑定。
	ListExpiredSignBinds(ctx context.Context, limit int) ([]*SignBind, error)
	// ── KeyRotDriftCase ───────────────────────────────────────────────────────
	// SaveDriftCase 新建漂移案件。
	SaveDriftCase(ctx context.Context, d *KeyRotDriftCase) error
	// GetDriftCase 按 drift_case_id 查询。
	GetDriftCase(ctx context.Context,
		id string) (*KeyRotDriftCase, error)
	// ListOpenDriftCases 查询指定源槽的 Open/Proven 案件。
	ListOpenDriftCases(ctx context.Context, srcSlotID string) ([]*KeyRotDriftCase, error)
	// UpdateDriftCaseState CAS 更新漂移案件状态。
	UpdateDriftCaseState(ctx context.Context, id string, from, to DriftCaseState, update *KeyRotDriftCase) error
	// ── FingerprintProof ──────────────────────────────────────────────────────
	// SaveProof 新建指纹证明记录。
	SaveProof(ctx context.Context, p *FingerprintProof) error
	// GetProof 按 proof_id 查询。
	GetProof(ctx context.Context, id string) (*FingerprintProof, error)
	// GetProofByLease 按 rotate_lease_id 查询（一个 Lease 对应一个 Proof）。
	GetProofByLease(ctx context.Context,
		leaseID string) (*FingerprintProof, error)
	// GetLatestProofBySlot 查询指定源槽最新的 FingerprintProof（按 issued_at 降序）。
	GetLatestProofBySlot(ctx context.Context, srcSlotID string) (*FingerprintProof, error)
}
