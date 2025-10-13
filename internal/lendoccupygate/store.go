package lendoccupygate

import (
	"context"
	"time"
)

// Store lendoccupygate 持久化接口。
type Store interface {
	// ── LendGen ──────────────────────────────────────────────────────────────
	// SaveGen 写入放贷世代令牌（insert-only，幂等：同 GenID 二次写忽略）。
	SaveGen(ctx context.Context, g *LendGen) error
	// GetGenByOrder 按 OrderID 查询 LendGen；未找到返回 ErrGenNotFound。
	GetGenByOrder(ctx context.Context,
		orderID string) (*LendGen, error)
	// GetGen 按 GenID 查询；未找到返回 ErrGenNotFound。
	GetGen(ctx context.Context,
		genID string) (*LendGen, error)
	// ConsumeGen CAS 消费：将 Consumed=false 的 gen 原子设为 Consumed=true。
	// 已消费返回 ErrGenConsumed；未找到返回 ErrGenNotFound。
	ConsumeGen(ctx context.Context, genID string, consumedBy string, at time.Time) error
	// ── LendOccupyLease ──────────────────────────────────────────────────────
	// SaveLease 新建占用租约（insert-only）。
	// 同 OrderID 已存在则返回 ErrGenAlreadyExists（防止重复 Place）。
	SaveLease(ctx context.Context, l *LendOccupyLease) error
	// GetLeaseByOrder 按 OrderID 查询租约；未找到返回 ErrLeaseNotFound。
	GetLeaseByOrder(ctx context.Context,
		orderID string) (*LendOccupyLease, error)
	// GetLease 按 LeaseID 查询；未找到返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*LendOccupyLease, error)
	// UpdateLeaseState CAS 更新租约状态（from→to），并可选更新 patch 字段。
	// 当前状态不等于 from 时返回 ErrInvalidTransition。
	UpdateLeaseState(ctx context.Context, leaseID string, from, to LeaseState, patch *LendOccupyLease) error
	// ListStalePlacedLeases 返回 placed_at < staleThreshold 且状态仍为 placed 的租约。
	// 供半截扫描器判断是否需要补偿冲销；limit <= 0 时默认 200。
	ListStalePlacedLeases(ctx context.Context, staleThreshold time.Time, limit int) ([]*LendOccupyLease, error)
	// ListLeasesByUser 返回指定用户的所有租约（按 placed_at 降序）；limit <= 0 时默认 50。
	ListLeasesByUser(ctx context.Context, userID string, limit int) ([]*LendOccupyLease, error)
	// ── LendDupReleaseProof ───────────────────────────────────────────────────
	// SaveDupProof 写入重复释放证明（insert-only）。
	// 同 GenID 已存在返回 ErrAlreadyProven（每个 gen 仅允许一条 dup 证明）。
	SaveDupProof(ctx context.Context, p *LendDupReleaseProof) error
	// GetDupProofByGen 按 GenID 查询重复释放证明；未找到返回 ErrDupProofNotFound。
	GetDupProofByGen(ctx context.Context, genID string) (*LendDupReleaseProof, error)
	// GetDupProof 按 ProofID 查询；未找到返回 ErrDupProofNotFound。
	GetDupProof(ctx context.Context, proofID string) (*LendDupReleaseProof, error)
	// ListDupProofsByOrder 按 OrderID 查询所有重复释放证明（一个 order 通常至多一条）。
	ListDupProofsByOrder(ctx context.Context, orderID string) ([]*LendDupReleaseProof, error)
	// ── LendResidueProof ──────────────────────────────────────────────────────
	// SaveResidueProof 写入半截补偿证明（insert-only）。
	// 同 GenID 已存在返回 ErrAlreadyProven（每个 gen 仅允许一条 residue 证明）。
	SaveResidueProof(ctx context.Context, p *LendResidueProof) error
	// GetResidueProofByGen 按 GenID 查询；未找到返回 ErrResidueProofNotFound。
	GetResidueProofByGen(ctx context.Context, genID string) (*LendResidueProof, error)
	// GetResidueProof 按 ProofID 查询；未找到返回 ErrResidueProofNotFound。
	GetResidueProof(ctx context.Context, proofID string) (*LendResidueProof, error)
	// ListResidueProofsByOrder 按 OrderID 查询所有半截补偿证明（通常至多一条）。
	ListResidueProofsByOrder(ctx context.Context, orderID string) ([]*LendResidueProof, error)
}
