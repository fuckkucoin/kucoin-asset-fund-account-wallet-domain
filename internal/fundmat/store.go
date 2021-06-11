package fundmat

import (
	"context"
	"time"
)

// ApplyOutcome 单事件应用结果。Applied 为假表示该标识已经物化过。
type ApplyOutcome struct {
	Applied bool
	Row MatRow
}

// Store 投影行、事件日志、水位、围栏、代次、重建任务与告警。
type Store interface {
	Append(ctx context.Context, ev FundingEvent) (FundingEvent, error)
	Pending(ctx context.Context, stream Stream, limit int) ([]FundingEvent, error)
	ApplyOne(ctx context.Context, ev FundingEvent, now time.Time) (ApplyOutcome, error)
	GetRow(ctx context.Context, user, asset, subject string) (MatRow, bool, error)
	PutRow(ctx context.Context, row MatRow) error
	ListUser(ctx context.Context, user string) ([]MatRow, error)
	ListUsers(ctx context.Context, limit int) ([]string, error)
	AppliedSeqs(ctx context.Context,
		stream Stream) ([]int64, error)
	HeadSeq(ctx context.Context, stream Stream) (int64, error)
	PutWM(ctx context.Context, wm MatWatermark) error
	ListWM(ctx context.Context) ([]MatWatermark,
		error)
	SealUserBalance(ctx context.Context, userID string, at time.Time) (int, error)
	PutFence(ctx context.Context, f ReadFence) error
	GetFence(ctx context.Context, id string) (ReadFence, error)
	PutVersion(ctx context.Context, v ViewVersion) error
	LatestVersion(ctx context.Context,
		userID string) (ViewVersion, error)
	PutJob(ctx context.Context, j RebuildJob) error
	GetJob(ctx context.Context, id string) (RebuildJob, error)
	ListJobs(ctx context.Context, status JobStatus, limit int) ([]RebuildJob, error)
	CasJob(ctx context.Context, id string, from JobStatus, next RebuildJob) (RebuildJob, error)
	PutAlert(ctx context.Context, a Alert) error
	ListAlerts(ctx context.Context, openOnly bool, limit int) ([]Alert, error)
	SeenSource(ctx context.Context, id string) (bool, error)
	MarkSource(ctx context.Context, id string, at time.Time) error
}
