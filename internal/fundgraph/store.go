package fundgraph

import (
	"context"
	"time"
)

// Store 是因果图的持久化。实现必须保证同边键的写入原子，冲突时不得覆盖。

type Store interface {
	Ingest(ctx context.Context,
		b IngestBatch) (IngestResult, error)
	GetNode(ctx context.Context, id string) (FundNode, bool, error)
	ListNodes(ctx context.Context, f NodeFilter) ([]FundNode, error)
	GetEdge(ctx context.Context, id string) (FundEdge, bool, error)
	EdgesFrom(ctx context.Context, nodeID string) ([]FundEdge, error)
	EdgesTo(ctx context.Context, nodeID string) ([]FundEdge, error)
	ListEdges(ctx context.Context,
		f EdgeFilter) ([]FundEdge, error)
	Counts(ctx context.Context) (nodes, edges int, err error)
	Snapshot(ctx context.Context) (Snapshot,
		error)
	PutCursor(ctx context.Context, c Cursor) error
	GetCursor(ctx context.Context, source, stream string) (Cursor, bool, error)
	ListCursors(ctx context.Context) ([]Cursor, error)
	OpenIncident(ctx context.Context, inc GraphIncident) (GraphIncident, bool, error)
	ListIncidents(ctx context.Context, f IncidentFilter) ([]GraphIncident, error)
	SetIncidentStatus(ctx context.Context, id, status string, at time.Time) error
	AppendAudit(ctx context.Context, a AuditRecord) error
	ListAudit(ctx context.Context, edgeKey string, limit int) ([]AuditRecord, error)
	SetEdgeTier(ctx context.Context, id, tier string, at time.Time) error
	DeleteEdges(ctx context.Context, ids []string) error
}
