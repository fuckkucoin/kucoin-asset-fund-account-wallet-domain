package wrfreeze

import (
	"context"
	"time"
)

// Store 持久化接口（所有写操作幂等/upsert）。

type Store interface {
	// ── CascadePlan ──
	SavePlan(ctx context.Context, plan *CascadePlan) error
	GetPlan(ctx context.Context,
		planID string) (*CascadePlan, error)
	ListPlans(ctx context.Context, status string, limit int) ([]*CascadePlan, error)
	ListOverduePlans(ctx context.Context, now time.Time) ([]*CascadePlan, error)
	// ── FreezeWave ──
	SaveWave(ctx context.Context, wave *FreezeWave) error
	GetWave(ctx context.Context, waveID string) (*FreezeWave, error)
	ListWaves(ctx context.Context,
		planID string) ([]*FreezeWave, error)
	SaveWaveEntry(ctx context.Context,
		entry *WaveUserEntry) error
	// ── UnfreezePhase ──
	SavePhase(ctx context.Context, phase *UnfreezePhase) error
	GetPhase(ctx context.Context,
		phaseID string) (*UnfreezePhase, error)
	ListPhases(ctx context.Context, planID string) ([]*UnfreezePhase, error)
	// ── CoverageProof ──
	SaveProof(ctx context.Context, proof *CoverageProof) error
	GetProof(ctx context.Context, proofID string) (*CoverageProof, error)
	LatestProof(ctx context.Context, planID string) (*CoverageProof, error)
	ListProofs(ctx context.Context,
		planID string) ([]*CoverageProof, error)
	// ── RollbackLeg ──
	SaveLeg(ctx context.Context, leg *RollbackLeg) error
	GetLeg(ctx context.Context, legID string) (*RollbackLeg, error)
	ListLegs(ctx context.Context, planID string) ([]*RollbackLeg, error)
}
