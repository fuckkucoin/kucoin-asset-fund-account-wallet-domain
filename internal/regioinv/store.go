package regioinv

import "context"

// Store 是库存域的持久化。业务规则在 Service，不在驱动里放宽。

type Store interface {
	SaveRegion(ctx context.Context, r Region) error
	GetRegion(ctx context.Context,
		id string) (Region, error)
	FindRegion(ctx context.Context, code string) (Region, error)
	ListRegions(ctx context.Context) ([]Region, error)
	SaveInv(ctx context.Context, row RegionInv) error
	FindInv(ctx context.Context, regionID, tier, asset, chain string) (RegionInv, error)
	ListInv(ctx context.Context, regionID string) ([]RegionInv, error)
	// Transfer 在同层两区之间划转可用，内部调用 NextPair 守恒验证。
	Transfer(ctx context.Context, t Transfer) (RegionInv, RegionInv, error)
	SaveMark(ctx context.Context, w Watermark) error
	FindMark(ctx context.Context, regionID, tier, asset, chain string) (Watermark, error)
	ListMarks(ctx context.Context,
		regionID string) ([]Watermark, error)
	AddUsed(ctx context.Context,
		regionID, tier, asset, chain, delta, day string, add bool) error
	SavePolicy(ctx context.Context, p IsoPolicy) error
	FindPolicy(ctx context.Context, regionID string) (IsoPolicy, error)
	SaveObs(ctx context.Context, o Observation) error
	ListObs(ctx context.Context, regionID, tier, asset, chain string) ([]Observation, error)
}
