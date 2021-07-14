package guardlease

import "time"

// LeaseState 租约状态机。open → bound → consumed | expired
type LeaseState string

const (
	LeaseOpen     LeaseState = "open"
	LeaseBound    LeaseState = "bound"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
)

// ImbalanceState 失衡记录状态机。open → proven → closed
type ImbalanceState string

const (
	ImbalanceOpen   ImbalanceState = "open"
	ImbalanceProven ImbalanceState = "proven"
	ImbalanceClosed ImbalanceState = "closed"
)

// ProofState 守恒证明状态机。pending → ok | fail → halted

type ProofState string

const (
	ProofPending ProofState = "pending"
	ProofOK     ProofState = "ok"
	ProofFail   ProofState = "fail"
	ProofHalted ProofState = "halted"
)

// DefaultLeaseTTL 租约默认生存期（快照到 Conserve 的最大允许窗口）。
const DefaultLeaseTTL = 30 * time.Second

// FourEyesClearDeadline 四眼解除超时后系统自动降级写阻塞。
const FourEyesClearDeadline = 10 * time.Minute

// SnapLease 快照世代租约。Gate/Snapshot 发放；Conserve 时携带并核验。
// 状态转移：
// open → bound : Conserve 携带合法租约调用 BindLease
// bound → consumed : ConserveProof 记录成功，ConsumedAt 落地
// open → expired : TTL 超期，SweepExpired worker 标记
// bound → expired : 未在 ExpiresAt 前完成 Conserve（不应发生，covered by fence）
type SnapLease struct {
	LeaseID    string `json:"lease_id"`
	Generation int64  `json:"generation"`
	UserID string `json:"user_id"`
	Asset        string     `json:"asset"`
	SnapshotHash string     `json:"snapshot_hash"`
	State        LeaseState `json:"state"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	BoundAt    *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ActorID    string     `json:"actor_id"`
}

// IsExpired 判断租约是否已超期。

   func (l SnapLease) IsExpired(now time.Time) bool {
	   return now.After(l.ExpiresAt)
   }

// IsUsable 租约是否可被 Conserve 携带（open 且未超期）。
  func (l SnapLease) IsUsable(now time.Time) bool {
	   return l.State == LeaseOpen && !l.IsExpired(now)
   }

// PartialBatch 部分批次摘要，升格后包含金额维度。
type PartialBatch struct {
	BatchID   string `json:"batch_id"`
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	Submitted int    `json:"submitted"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
	Total   int `json:"total"`
	SubmittedAmt string `json:"submitted_amt"`
	SkippedAmt string `json:"skipped_amt"`
	FailedAmt string `json:"failed_amt"`
	ExpectedTotal string `json:"expected_total"`
	ActualTotal string `json:"actual_total"`
}

// PartialImbalance 部分批次金额失衡证明。
// 状态转移：
// open → proven : 双人确认后 Prove(actor1, actor2) 调用
// proven → closed : ops 手动或自动巡检关闭（已修复）
type PartialImbalance struct {
	ImbalanceID string `json:"imbalance_id"`
	BatchID string `json:"batch_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	ExpectedTotal string `json:"expected_total"`
	ActualTotal   string `json:"actual_total"`
	Delta string `json:"delta"`
	State      ImbalanceState `json:"state"`
	DetectedAt time.Time      `json:"detected_at"`
	ProvenAt   *time.Time     `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	Actor1 string `json:"actor1,omitempty"`
	Actor2 string `json:"actor2,omitempty"`
}

// ConserveProof 守恒证明：记录 Gate 前后 Bundle 核对，失败升级 halt。
// 状态转移：
// pending → ok : before/after 金额守恒通过
// pending → fail : 守恒偏差，挡写直至四眼解除
// fail → halted : 超过 FourEyesClearDeadline 未解除

type ConserveProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Asset       string `json:"asset"`
	BeforeTotal string `json:"before_total"`
	AfterTotal  string `json:"after_total"`
	SnapHash    string `json:"snap_hash"`
	State      ProofState `json:"state"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
	ClearedBy string `json:"cleared_by,omitempty"`
	EscalatedAt *time.Time `json:"escalated_at,omitempty"`
}

// GuardGeneration 守恒闸世代计数器，单调递增，每次 Gate/Snapshot 递增。
type GuardGeneration struct {
	UserID     string    `json:"user_id"`
	Asset      string    `json:"asset"`
	Generation int64     `json:"generation"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// FenceRequest 活账写请求在租约窗口内的排队凭证。
type FenceRequest struct {
	FenceID string `json:"fence_id"`
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Kind    string `json:"kind"`
	Amount  string `json:"amount"`
	QueuedAt  time.Time  `json:"queued_at"`
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
}

// IssueLeaseRequest Gate/Snapshot 调用时的参数。
type IssueLeaseRequest struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	SnapshotHash string `json:"snapshot_hash"`
	ActorID string        `json:"actor_id"`
	TTL     time.Duration `json:"ttl,omitempty"`
}

// BindLeaseRequest Conserve 时携带的租约核验参数。

type BindLeaseRequest struct {
	LeaseID string `json:"lease_id"`
	UserID       string `json:"user_id"`
	Asset        string `json:"asset"`
	SnapshotHash string `json:"snapshot_hash"`
}

// CheckPartialAmtRequest 升格后的部分批次守恒请求。
type CheckPartialAmtRequest struct {
	BatchID string `json:"batch_id"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Submitted int `json:"submitted"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
	Total        int    `json:"total"`
	SubmittedAmt string `json:"submitted_amt"`
	SkippedAmt   string `json:"skipped_amt"`
	FailedAmt    string `json:"failed_amt"`
	ExpectedTotal string `json:"expected_total"`
}

// ConserveProofRequest 记录守恒证明。
type ConserveProofRequest struct {
	LeaseID string `json:"lease_id"`
	UserID string `json:"user_id"`
	Asset       string `json:"asset"`
	BeforeTotal string `json:"before_total"`
	AfterTotal  string `json:"after_total"`
	SnapHash string `json:"snap_hash"`
}

// FourEyesClearRequest 四眼解除 ConserveProof fail 挡写。
type FourEyesClearRequest struct {
	ProofID string `json:"proof_id"`
	Actor1 string `json:"actor1"`
	Actor2 string `json:"actor2"`
}
