package solvency

import "time"

// 证明单状态。building 只表示采集未完成，不得对外当作根。

type Status string

const (
	StatusBuilding Status = "building"
	StatusSealed Status = "sealed"
	StatusPublished Status = "published"
	StatusRevoked   Status = "revoked"
	StatusAbandoned Status = "abandoned"
)

// 漂移工单状态。短缺必须先确认再关闭，不能从 open 直接结案。
type DriftStatus string

const (
	DriftOpen  DriftStatus = "open"
	DriftAcked DriftStatus = "acked"
	DriftClosed DriftStatus = "closed"
)

const (
	SevWatch    = "watch"
	SevShortage = "shortage"
	SevDeficit  = "deficit"
)

const (
	ClassInternal = "solvency_internal"
	ClassPublishable = "solvency_publishable"
	ClassEngineering = "engineering_merkle"
	CoverageCovered  = "covered"
	CoverageShort  = "short"
	CoverageUnread = "unreadable"
	SignerID       = "solvency-desk"
	leafPrefix = "solvency-leaf-v1"
	sealPrefix    = "solvency-seal-v1"
	sigPrefix     = "solvency-sig-v1"
	SourceCustody = "custody_books"
	SourceOverride = "observation_override"
	SourceUnreadable      = "unreadable"
	maxWindow             = 24 * time.Hour
	accountScanLimit      = 10000
	fenceScanLimit        = 500
	DisclaimerInternal    = "偿付证明（内控）：负债根已准备金；存在短缺、范围裁剪或未封存时不得当作已覆盖的可发布档"
	DisclaimerPublishable = "偿付证明（可发布档）：观察准备金在容差内覆盖全部用户负债；不是工程默克尔免责快照"
)

// LiabilityLeaf 一名用户、一个科目、一种资产在某一围栏代次上的负债。
// 金额只接受账本十进制字符串。叶哈希提交的是该围栏上的可用与冻结，不是实时账面。

type LiabilityLeaf struct {
	ID string `json:"id"`
	AttestationID string `json:"attestation_id"`
	UserID        string `json:"user_id"`
	Book          string `json:"book"`
	Asset         string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Total     string `json:"total"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	LeafHash    string    `json:"leaf_hash"`
	CollectedAt time.Time `json:"collected_at"`
}

// ReserveSnapshot 某一资产的热、温、冷观察合计，以及与用户负债的差额。
// 观察失败时不得把观察记成 0 后接着声称已覆盖。

type ReserveSnapshot struct {
	ID            string `json:"id"`
	AttestationID string `json:"attestation_id"`
	Asset string `json:"asset"`
	Hot   string `json:"hot"`
	Warm  string `json:"warm"`
	Cold string `json:"cold"`
	Observed  string `json:"observed"`
	Liability string `json:"liability"`
	Surplus string `json:"surplus"`
	Deficit string `json:"deficit"`
	Source  string    `json:"source"`
	TakenAt time.Time `json:"taken_at"`
}

// Attestation 一轮偿付证明。Root 在 building 期间只是草稿，SealDigest 为空则禁止发布。

type Attestation struct {
	ID string `json:"id"`
	ScheduleID string `json:"schedule_id,omitempty"`
	Status   Status `json:"status"`
	Class    string `json:"class"`
	Solvency bool   `json:"solvency"`
	Deprecated bool   `json:"deprecated"`
	Root       string `json:"root"`
	LeafCount   int       `json:"leaf_count"`
	Signature   string    `json:"signature,omitempty"`
	Signer      string    `json:"signer,omitempty"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	BookDigest string `json:"book_digest"`
	SealDigest string `json:"seal_digest,omitempty"`
	Coverage string `json:"coverage,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Disclaimer string `json:"disclaimer"`
	Reason   string `json:"reason,omitempty"`
	AuditRef string `json:"audit_ref,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SealedAt    *time.Time `json:"sealed_at,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// CausalRef 只携带证明单号，供调用方在因果边上引用。本包不落因果图。

type CausalRef struct {
	AttestationID string `json:"attestation_id"`
	EdgeKind string `json:"edge_kind"`
}

  func (a Attestation) CausalRef() CausalRef {
	  return CausalRef{AttestationID: a.ID, EdgeKind: "solvency_root"}
   }

   func (a Attestation) Scoped() bool { return a.Scope != "" }

// DriftCase 准备金低于用户负债时的工单。Deficit、RatioBps、Severity、Explain 必须同时可读。
type DriftCase struct {
	ID string `json:"id"`
	AttestationID string      `json:"attestation_id"`
	Asset         string      `json:"asset"`
	Status        DriftStatus `json:"status"`
	Severity      string      `json:"severity"`
	Liability string `json:"liability"`
	Observed  string `json:"observed"`
	Hot       string `json:"hot"`
	Warm string `json:"warm"`
	Cold string `json:"cold"`
	Deficit  string `json:"deficit"`
	RatioBps int64  `json:"ratio_bps"`
	Explain  string `json:"explain"`
	OpenedAt time.Time `json:"opened_at"`
	AckedAt  *time.Time `json:"acked_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	AckedBy string `json:"acked_by,omitempty"`
	ClosedBy string `json:"closed_by,omitempty"`
	Note string `json:"note,omitempty"`
	Signaled bool `json:"signaled"`
}

// Schedule 调度窗。Window 是封存截止（T+），超过后只能废弃 building，不能补封成根。

type Schedule struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Interval time.Duration `json:"interval"`
	Window   time.Duration `json:"window"`
	Assets []string `json:"assets,omitempty"`
	Enabled     bool `json:"enabled"`
	AutoPublish bool `json:"auto_publish"`
	NextDue   time.Time `json:"next_due"`
	LastRunID string    `json:"last_run_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AttestPatch 状态迁移允许改的字段。根、叶数、围栏与账面摘要不在补丁里，避免发布时改写草稿根。
type AttestPatch struct {
	Status Status
	Signature string
	Signer string
	SealDigest string
	Class      string
	Disclaimer string
	Coverage   string
	Reason     string
	AuditRef   string
	SealedAt *time.Time
	PublishedAt *time.Time
	RevokedAt *time.Time
	UpdatedAt time.Time
}

// ReserveLeg 热、温、冷三段观察。热对应 system.bridge，温对应 warm，冷对应 cold。
type ReserveLeg struct {
	Asset string `json:"asset"`
	Hot  string `json:"hot"`
	Warm string `json:"warm"`
	Cold string `json:"cold"`
	Source string `json:"source,omitempty"`
}

// PatrolReport 巡检结果。废弃的 building 不会出现在 Published 里。
type PatrolReport struct {
	At        time.Time `json:"at"`
	Abandoned []string  `json:"abandoned,omitempty"`
	Findings  []Finding `json:"findings,omitempty"`
}

type Finding struct {
	AttestationID string `json:"attestation_id"`
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

// DriftSignal 发往资金总线的短缺信号。只描述工单，不携带充提状态迁移。

type DriftSignal struct {
	DriftID       string `json:"drift_id"`
	AttestationID string `json:"attestation_id"`
	Asset    string `json:"asset"`
	Severity string `json:"severity"`
	Liability string `json:"liability"`
	Observed  string `json:"observed"`
	Deficit   string `json:"deficit"`
	RatioBps int64 `json:"ratio_bps"`
	At time.Time `json:"at"`
	Topic string `json:"topic"`
}
