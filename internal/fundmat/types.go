package fundmat

import (
	"strings"
	"time"
)

// Stream 投影订阅的资金事实流。序号在流内独立，不与偿付叶序号混用。
type Stream string

const (
	StreamLedger    Stream = "ledger"
	StreamFreeze    Stream = "freeze"
	StreamOccupancy Stream = "occupancy"
)

// Streams 水位巡检与追平判断的固定顺序。
  func Streams() []Stream {
	  return []Stream{StreamLedger, StreamFreeze, StreamOccupancy}
  }

   func (s Stream) Valid() bool {
	   switch s {
	   case StreamLedger, StreamFreeze, StreamOccupancy:
		   return true
	   default:
		  return false
	  }
  }

// Op 单行物化操作。冻结是可用与冻结的成对搬家，不是单边加计。

type Op string

const (
	OpFreeze          Op = "freeze"
	OpUnfreeze        Op = "unfreeze"
	OpCreditAvailable Op = "credit_available"
	OpDebitAvailable Op = "debit_available"
	OpDebitFrozen    Op = "debit_frozen"
	OpInFlightOpen   Op = "inflight_open"
	OpInFlightClose Op = "inflight_close"
	OpOccupy  Op = "occupy"
	OpRelease Op = "release"
	OpSetAvailable Op = "set_available"
	OpSetFrozen    Op = "set_frozen"
)

  func (o Op) Valid() bool {
	  switch o {
	   case OpFreeze, OpUnfreeze, OpCreditAvailable, OpDebitAvailable, OpDebitFrozen,
		  OpInFlightOpen, OpInFlightClose, OpOccupy, OpRelease, OpSetAvailable, OpSetFrozen:
		  return true
	  default:
		   return false
	   }
   }

// StreamOf 操作默认落入的事实流。调用方显式指定流时以调用方为准。

   func StreamOf(op Op) Stream {
	   switch op {
	  case OpFreeze, OpUnfreeze:
		   return StreamFreeze
	   case OpOccupy, OpRelease:
		   return StreamOccupancy
	   default:
		   return StreamLedger
	  }
  }

// AffectsBalance 为真时，全量重建可以用账本绝对余额覆盖该事件，避免再叠加一次增量。
  func (o Op) AffectsBalance() bool {
	   switch o {
	  case OpFreeze, OpUnfreeze, OpCreditAvailable, OpDebitAvailable, OpDebitFrozen, OpSetAvailable, OpSetFrozen:
		   return true
	   default:
		   return false
	  }
  }

// Bucket 在途与占用的分栏。合计栏由分栏重算，不单独接受增量。
type Bucket string

const (
	BucketDeposit  Bucket = "deposit"
	BucketWithdraw Bucket = "withdraw"
	BucketTransfer Bucket = "transfer"
	BucketRisk    Bucket = "risk"
	BucketReserve Bucket = "reserve"
)

  func (b Bucket) Valid() bool {
	   switch b {
	   case BucketDeposit, BucketWithdraw, BucketTransfer, BucketRisk, BucketReserve, "":
		  return true
	  default:
		  return false
	   }
  }

// MatRow 用户×资产×科目的物化行。在途与占用只供读面展示，不反向写账本。

type MatRow struct {
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Subject string `json:"subject"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	InFlight string `json:"in_flight"`
	InFlightDeposit  string `json:"in_flight_deposit"`
	InFlightWithdraw string `json:"in_flight_withdraw"`
	InFlightTransfer string `json:"in_flight_transfer"`
	Occupancy string `json:"occupancy"`
	OccupancyRisk    string    `json:"occupancy_risk"`
	OccupancyReserve string    `json:"occupancy_reserve"`
	CausalID         string    `json:"causal_id,omitempty"`
	LastEventID      string    `json:"last_event_id,omitempty"`
	RowVersion       int64     `json:"row_version"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Key 行主键。资产大写、科目小写，避免同一余额拆成两行。

   func (r MatRow) Key() string {
	  return RowKey(r.UserID, r.Asset, r.Subject)
  }

// MatWatermark 一条事实流的追平水位。强一致读比较连续序号，不比较已应用的最大序号。

type MatWatermark struct {
	Stream Stream `json:"stream"`
	Epoch         int64 `json:"epoch"`
	HeadSeq       int64 `json:"head_seq"`
	ContiguousSeq int64 `json:"contiguous_seq"`
	AppliedCount int64 `json:"applied_count"`
	HoleCount int64 `json:"hole_count"`
	LagEvents int64 `json:"lag_events"`
	LagMs     int64 `json:"lag_ms"`
	SLAMs     int64 `json:"sla_ms"`
	WithinSLA bool `json:"within_sla"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CaughtUp 连续水位已经覆盖日志头。
   func (w MatWatermark) CaughtUp() bool {
	   return w.ContiguousSeq >= w.HeadSeq
  }

// ReadFence 一次强一致读要求的水位。投影连续序号低于 MinSeq 时拒绝读。
type ReadFence struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Stream Stream `json:"stream,omitempty"`
	Epoch  int64 `json:"epoch"`
	MinSeq int64 `json:"min_seq"`
	Token    string    `json:"token"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Expired 过期围栏不得再当作强一致依据。

   func (f ReadFence) Expired(now time.Time) bool {
	  if f.ExpiresAt.IsZero() {
		  return false
	   }
	   return !now.Before(f.ExpiresAt)
  }

// ViewVersion 一次物化视图的代次与摘要。摘要覆盖排序后的分栏，供客服。

type ViewVersion struct {
	ID string `json:"id"`
	Scope string `json:"scope"`
	UserID   string `json:"user_id"`
	Version  int64  `json:"version"`
	Epoch    int64  `json:"epoch"`
	WMSeq    int64  `json:"wm_seq"`
	Digest   string `json:"digest"`
	RowCount int    `json:"row_count"`
	BuiltAt time.Time `json:"built_at"`
}

// JobStatus 重建任务状态。只允许 queued→running→done 或 queued→running→failed。

type JobStatus string

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed JobStatus = "failed"
)

   func (s JobStatus) Valid() bool {
	  switch s {
	  case JobQueued,
		  JobRunning, JobDone, JobFailed:
		   return true
	  default:
		  return false
	   }
   }

// Active 排队或执行中的任务占用该用户的重建槽。
   func (s JobStatus) Active() bool {
	  return s == JobQueued || s == JobRunning
  }

// RebuildJob 按用户或全量把可用/冻结栏重放到账本绝对余额。
type RebuildJob struct {
	ID         string    `json:"id"`
	Scope      string    `json:"scope"`
	UserID     string    `json:"user_id"`
	Status     JobStatus `json:"status"`
	Tolerance  string    `json:"tolerance"`
	DriftCount int       `json:"drift_count"`
	DriftSample string `json:"drift_sample,omitempty"`
	Error       string `json:"error,omitempty"`
	QueuedAt  time.Time  `json:"queued_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// FundingEvent 泵消费的一条资金事实。同一 ID 重复投递不得改变分栏。
type FundingEvent struct {
	ID string `json:"id"`
	Stream Stream `json:"stream"`
	Seq    int64  `json:"seq"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Subject string `json:"subject"`
	Op       Op     `json:"op"`
	Amount   string `json:"amount"`
	Bucket   Bucket `json:"bucket,omitempty"`
	CausalID string `json:"causal_id,omitempty"`
	SourceEventID string `json:"source_event_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Alert 水位落后或投影漂移。只记录在物化告警表，不写入偿付漂移工单。
type Alert struct {
	ID string `json:"id"`
	Kind     string    `json:"kind"`
	Stream   string    `json:"stream,omitempty"`
	UserID   string    `json:"user_id,omitempty"`
	Detail   string    `json:"detail"`
	Open     bool      `json:"open"`
	RaisedAt time.Time `json:"raised_at"`
}

// BookLine 账本只读余额，重建与漂移的输入。
type BookLine struct {
	UserID string `json:"user_id"`
	Asset   string `json:"asset"`
	Subject string `json:"subject"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
}

// Drift 某一分栏与账本不一致。
type Drift struct {
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Subject string `json:"subject"`
	Field   string `json:"field"`
	Ledger string `json:"ledger"`
	Projection string `json:"projection"`
}

// ApplyStats 一轮泵的计数。
type ApplyStats struct {
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
	Rejected int  `json:"rejected"`
	Paused   bool `json:"paused"`
}

// LagReport 各流追平情况，供读围栏与巡检共用。

type LagReport struct {
	CaughtUp bool `json:"caught_up"`
	MaxLagEvents int64 `json:"max_lag_events"`
	MaxLagMs     int64 `json:"max_lag_ms"`
	SLAMs        int64 `json:"sla_ms"`
	WithinSLA    bool  `json:"within_sla"`
	Paused     bool           `json:"paused"`
	Streams    []MatWatermark `json:"streams"`
	OpenAlerts int            `json:"open_alerts"`
}

  func NormAsset(s string) string {
	  return strings.ToUpper(strings.TrimSpace(s))
   }

  func NormSubject(s string) string {
	   return strings.ToLower(strings.TrimSpace(s))
  }

  func NormUser(s string) string {
	  return strings.TrimSpace(s)
   }

// RowKey 物化主键。

   func RowKey(user, asset, subject string) string {
	   return NormUser(user) + "|" + NormAsset(asset) + "|" + NormSubject(subject)
   }

// BlankRow 全零行。新建投影时从这里起算，禁止缺省空串被当成非法金额。
   func BlankRow(user, asset, subject string, now time.Time) MatRow {
	  z := "0"
	   return MatRow{
		   UserID: NormUser(user), Asset: NormAsset(asset), Subject: NormSubject(subject),
		   Available: z,
		  Frozen:    z, InFlight: z,
		  InFlightDeposit: z, InFlightWithdraw: z, InFlightTransfer: z,
		  Occupancy: z, OccupancyRisk: z, OccupancyReserve: z,
		  UpdatedAt: now.UTC(),
	  }
  }
