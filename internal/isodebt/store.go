package isodebt

import (
	"context"
	"time"
)

// Store 持久化接口——所有领域写入必须经此接口。
type Store interface {
	// ── PairGeneration ──────────────────────────────────
	// GetGeneration 读取最新世代；不存在时返回 zero-value PairGeneration（Generation=0）。
	GetGeneration(ctx context.Context,
		userID, symbol string) (PairGeneration, error)
	// SaveGeneration 持久化世代（upsert by userID+symbol）。
	SaveGeneration(ctx context.Context, g PairGeneration) error
	// ListGenerations 返回近 limit 条世代历史（按 ComputedAt desc）。
	ListGenerations(ctx context.Context, userID, symbol string, limit int) ([]PairGeneration, error)
	// ── RatioLease ──────────────────────────────────────
	// GetLease 按 ID 读取。
	GetLease(ctx context.Context,
		id string) (RatioLease, error)
	// SaveLease 持久化（insert or update）。
	SaveLease(ctx context.Context, l RatioLease) error
	// ListActiveLeases 返回 userID+symbol 的 fresh 租约（ExpiresAt > now）。
	ListActiveLeases(ctx context.Context, userID, symbol string, now time.Time) ([]RatioLease, error)
	// ExpireStaleLeases 将世代不符或超时的 fresh 租约标记为 stale。
	ExpireStaleLeases(ctx context.Context,
		userID, symbol string, currentGen int64, now time.Time) (int64, error)
	// ── TransferGate ─────────────────────────────────────
	// GetGate 按 ID 读取。
	GetGate(ctx context.Context, id string) (TransferGate, error)
	// SaveGate 持久化。
	SaveGate(ctx context.Context,
		g TransferGate) error
	// ListPendingGates 返回 pending 状态的转入门（用于巡检）。
	ListPendingGates(ctx context.Context, limit int) ([]TransferGate, error)
	// ── RatioDriftCase ───────────────────────────────────
	// GetDriftCase 按 ID 读取。
	GetDriftCase(ctx context.Context, id string) (RatioDriftCase, error)
	// SaveDriftCase 持久化。
	SaveDriftCase(ctx context.Context, d RatioDriftCase) error
	// ListOpenDriftCases 返回 open 状态的漂移案例。
	ListOpenDriftCases(ctx context.Context, userID, symbol string) ([]RatioDriftCase, error)
	// ListAllOpenDriftCases 全量扫描 open 案例（用于漂移巡检 worker）。
	ListAllOpenDriftCases(ctx context.Context, limit int) ([]RatioDriftCase, error)
	// ── RatioDriftProof ──────────────────────────────────
	// SaveProof 持久化证明。
	SaveProof(ctx context.Context, p RatioDriftProof) error
	// GetProof 按 ID 读取。
	GetProof(ctx context.Context, id string) (RatioDriftProof, error)
	// ListProofsByCase 返回某案例下的所有证明。
	ListProofsByCase(ctx context.Context, driftCaseID string) ([]RatioDriftProof, error)
	// ── MutexRecord ──────────────────────────────────────
	// SaveMutexRecord 持久化冲突记录。
	SaveMutexRecord(ctx context.Context, m MutexRecord) error
	// ListMutexRecords 返回近 limit 条冲突记录。
	ListMutexRecords(ctx context.Context,
		userID, symbol string, limit int) ([]MutexRecord, error)
}
