package airimportgate

import "context"

// Store airimportgate 持久化接口。
type Store interface {
	// ── ImportLease ──────────────────────────────────────────────────────────
	// SaveLease 新建租约（insert-only）。
	SaveLease(ctx context.Context, l *ImportLease) error
	// GetLease 按 lease_id 查询。
	GetLease(ctx context.Context, id string) (*ImportLease, error)
	// UpdateLeaseState CAS 更新租约状态（from→to）；状态不符返回 ErrInvalidTransition。
	UpdateLeaseState(ctx context.Context,
		id string, from, to LeaseState, update *ImportLease) error
	// ListActiveLeasesForSession 查询指定 Session 的 open/importing 租约（至多一条）。
	ListActiveLeasesForSession(ctx context.Context, sessionID string) ([]*ImportLease, error)
	// ListExpiredLeases 查询已超期且非终态的租约。
	ListExpiredLeases(ctx context.Context, limit int) ([]*ImportLease, error)
	// ListLeasesBySession 查询 Session 下的所有租约（分页）。
	ListLeasesBySession(ctx context.Context, sessionID string, limit int) ([]*ImportLease, error)
	// ── SealedBlobDigest ─────────────────────────────────────────────────────
	// SaveDigest 新建摘要记录；DigestHex 冲突返回 ErrBlobReplay。
	SaveDigest(ctx context.Context,
		d *SealedBlobDigest) error
	// GetDigestByHex 按 sha256 hex 查询摘要记录；未找到返回 ErrDigestNotFound。
	GetDigestByHex(ctx context.Context, hex string) (*SealedBlobDigest, error)
	// ListDigestsBySession 查询 Session 下的所有摘要记录。
	ListDigestsBySession(ctx context.Context, sessionID string) ([]*SealedBlobDigest, error)
	// ── PhaseConflictProof ───────────────────────────────────────────────────
	// SaveConflict 新建冲突证明。
	SaveConflict(ctx context.Context,
		p *PhaseConflictProof) error
	// GetConflict 按 conflict_id 查询。
	GetConflict(ctx context.Context, id string) (*PhaseConflictProof, error)
	// UpdateConflictState CAS 更新冲突状态。
	UpdateConflictState(ctx context.Context, id string, from, to ConflictState, update *PhaseConflictProof) error
	// ListOpenConflictsBySession 查询 Session 下 open/proven 的冲突证明。
	ListOpenConflictsBySession(ctx context.Context, sessionID string) ([]*PhaseConflictProof, error)
	// ── QuorumBind ───────────────────────────────────────────────────────────
	// SaveBind 新建法定人数绑定记录（insert-only）。
	SaveBind(ctx context.Context, b *QuorumBind) error
	// GetBind 按 bind_id 查询。
	GetBind(ctx context.Context, id string) (*QuorumBind, error)
	// GetBindByLease 按 lease_id 查询（一个 Lease 对应一个 Bind）。
	GetBindByLease(ctx context.Context, leaseID string) (*QuorumBind, error)
}
