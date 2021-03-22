package auditchain

import (
	"context"
	"time"
)

const (
	SegOpen   = "open"
	SegSealed = "sealed"
	SegFrozen = "frozen"
	TierHot  = "hot"
	TierCold = "cold"
	JobQueued  = "queued"
	JobRunning = "running"
	JobDone   = "done"
	JobFailed = "failed"
	ExportPending  = "pending"
	ExportReleased = "released"
	ExportDenied = "denied"
	AlertBroken = "audit.chain.broken"
	AlertMissing = "audit.segment.missing"
	AlertDrift   = "audit.checkpoint.drift"
	AlertLag     = "audit.ingest.lag"
	AlertLate = "audit.ingest.late"
	AlertBlocked = "audit.export.blocked"
	AlertReplay  = "audit.replay.diff"
	AlertStrict = "audit.ingest.strict"
)

// Transit 段状态。相等视为已在目标态。其余只允许封存和冻结两步。
   func Transit(from, to string) error {
	   if from == to {
		   return nil
	   }
	  if from == SegOpen && to == SegSealed {
		   return nil
	  }
	  if from == SegSealed && to == SegFrozen {
		  return nil
	  }
	  if to == SegFrozen {
		  return ErrFrozen
	   }
	  if to == SegSealed {
		  return ErrSealed
	  }
	   return ErrSegment
  }

type Segment struct {
	ID string `json:"id"`
	PartKey string `json:"part_key"`
	Domain string `json:"domain"`
	Tenant  string `json:"tenant"`
	DayKey  string `json:"day_key"`
	Ordinal int    `json:"ordinal"`
	State string `json:"state"`
	Tier  string `json:"tier"`
	Count int    `json:"entry_count"`
	HeadHash string `json:"head_hash"`
	TailHash string `json:"tail_hash"`
	MerkleRoot string `json:"merkle_root"`
	HoldReason string    `json:"hold_reason,omitempty"`
	OpenedAt   time.Time `json:"opened_at"`
	SealedAt time.Time `json:"sealed_at,omitempty"`
	FrozenAt time.Time `json:"frozen_at,omitempty"`
}

type Row struct {
	ID string `json:"id"`
	SegmentID string `json:"segment_id"`
	Seq       int    `json:"seq"`
	IdemKey string `json:"idem_key"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Domain string `json:"domain"`
	Tenant string `json:"tenant"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	RefID  string `json:"ref_id"`
	Actor  string `json:"actor"`
	Body     string `json:"body"`
	PrevHash string `json:"prev_hash"`
	SelfHash   string    `json:"self_hash"`
	OccurredAt time.Time `json:"occurred_at"`
	IngestedAt time.Time `json:"ingested_at"`
	Duplicate bool `json:"duplicate,omitempty"`
}

type Checkpoint struct {
	ID string `json:"id"`
	SegmentID  string `json:"segment_id"`
	Seq        int    `json:"seq"`
	MerkleRoot string `json:"merkle_root"`
	PrevRoot string `json:"prev_root"`
	Count    int    `json:"entry_count"`
	Tip bool `json:"tip"`
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}

type Cursor struct {
	Source string `json:"source"`
	HighWater string        `json:"high_water"`
	LastID    string        `json:"last_id"`
	Lag       time.Duration `json:"lag"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Alert struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SegmentID string `json:"segment_id,omitempty"`
	Detail string `json:"detail"`
	Open bool      `json:"open"`
	At   time.Time `json:"at"`
}

type Job struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset string    `json:"asset"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	State string `json:"state"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	DualID     string `json:"dual_id,omitempty"`
	ReportJSON string `json:"report_json,omitempty"`
	ErrorText  string `json:"error_text,omitempty"`
	Actor string `json:"actor"`
	Expect    []string  `json:"expect,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ExportRec struct {
	ID        string `json:"id"`
	SegmentID string `json:"segment_id"`
	UserID string `json:"user_id"`
	Asset string    `json:"asset"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	ApprovalID  string `json:"approval_id"`
	RequestedBy string `json:"requested_by"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
}

// Store 只追加仓储。改历史的方法只有演练入口， HTTP 不挂。
type Store interface {
	FindIdem(ctx context.Context,
		source, key string) (Row, bool, error)
	Segments(ctx context.Context, partKey string) ([]Segment, error)
	AllSegments(ctx context.Context) ([]Segment,
		error)
	Segment(ctx context.Context, id string) (Segment, bool, error)
	CreateSegment(ctx context.Context, s Segment) error
	SaveSegment(ctx context.Context,
		s Segment) error
	InsertRow(ctx context.Context,
		r Row) error
	Rows(ctx context.Context, segmentID string) ([]Row, error)
	UserRows(ctx context.Context, user, asset string, from, to time.Time) ([]Row, error)
	PutCheckpoint(ctx context.Context, c Checkpoint) error
	Checkpoints(ctx context.Context, segmentID string) ([]Checkpoint, error)
	AllCheckpoints(ctx context.Context) ([]Checkpoint,
		error)
	Cursor(ctx context.Context, source string) (Cursor, bool, error)
	PutCursor(ctx context.Context, c Cursor) error
	Cursors(ctx context.Context) ([]Cursor, error)
	PutAlert(ctx context.Context, a Alert) error
	Alerts(ctx context.Context, openOnly bool) ([]Alert, error)
	PutJob(ctx context.Context,
		j Job) error
	Job(ctx context.Context, id string) (Job, bool, error)
	Jobs(ctx context.Context, state string) ([]Job, error)
	PutExport(ctx context.Context,
		e ExportRec) error
	Export(ctx context.Context, id string) (ExportRec, bool, error)
	CorruptBody(ctx context.Context, segmentID string, seq int, body string) error
	RewriteRoot(ctx context.Context, segmentID, root string) error
}
