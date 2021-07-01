package georebal

import "context"

// Store 是再平衡计划的持久化。不写证明链。
type Store interface {
	SavePlan(ctx context.Context, p GeoRebalPlan) error
	GetPlan(ctx context.Context, id string) (GeoRebalPlan, error)
	FindPlanByIdem(ctx context.Context, idem string) (GeoRebalPlan, error)
	ListPlans(ctx context.Context, states []string) ([]GeoRebalPlan, error)
	SaveLeg(ctx context.Context, leg GeoRebalLeg) error
	GetLeg(ctx context.Context, id string) (GeoRebalLeg, error)
	ListLegs(ctx context.Context, planID string) ([]GeoRebalLeg, error)
	UpdateLegState(ctx context.Context,
		id, state, sweepIntentID, reason string) error
}
