package occupbook

import "context"

// Store 占用账本仓储。
type Store interface {
	SaveLot(ctx context.Context, lot *Lot) error
	GetLot(ctx context.Context, lotID string) (*Lot, error)
	UpdateLot(ctx context.Context, lot *Lot) error
	ListOpenLots(ctx context.Context,
		limit int) ([]*Lot, error)
	SaveHold(ctx context.Context, h *Hold) error
	GetHold(ctx context.Context, holdID string) (*Hold, error)
	GetHoldByIdem(ctx context.Context, lotID, idemKey string) (*Hold, error)
	UpdateHold(ctx context.Context, h *Hold) error
	ListHoldsByLot(ctx context.Context, lotID string) ([]*Hold, error)
	SaveRelease(ctx context.Context,
		r *Release) error
	GetRelease(ctx context.Context, releaseID string) (*Release, error)
	GetReleaseByIdem(ctx context.Context, lotID, idemKey string) (*Release, error)
	SaveReclass(ctx context.Context,
		r *Reclass) error
	GetReclass(ctx context.Context, reclassID string) (*Reclass, error)
	GetReclassByIdem(ctx context.Context, lotID, idemKey string) (*Reclass, error)
	AppendAudit(ctx context.Context, ev *AuditEvent) error
	ListAudit(ctx context.Context,
		lotID string, limit int) ([]*AuditEvent, error)
}
