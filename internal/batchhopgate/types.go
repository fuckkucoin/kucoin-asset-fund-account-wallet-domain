package batchhopgate

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// ResidualStatus — BatchResidualCase 状态枚举
// ─────────────────────────────────────────────────────────────────────────────
// ResidualStatus 表示残留案的生命周期状态。
type ResidualStatus string

const (
	// ResidualOpen 残留案已落库，尚未附占用证明。禁止同用户同资产发起并行 BatchHop。
	ResidualOpen ResidualStatus = "open"
	// ResidualProven 占用证明已生成并附于残留案。可进入续跳流程。
	ResidualProven ResidualStatus = "proven"
	// ResidualResuming 续跳 Resume 正在推进（ResumeLease 已绑定）。
	ResidualResuming ResidualStatus = "resuming"
	// ResidualClosed 续跳已完成或人工收口，残留案关闭。
	ResidualClosed ResidualStatus = "closed"
	// ResidualBlocked 因与他包世代冲突或审核拒绝，残留案进入阻塞态。
	ResidualBlocked ResidualStatus = "blocked"
)

// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus — ResumeLease 状态枚举
// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus 表示续跳互斥租约的生命周期状态。
type LeaseStatus string

const (
	// LeaseOpen 续跳令牌已颁发，等待绑定具体续跳操作。
	LeaseOpen LeaseStatus = "open"
	// LeaseBound 续跳令牌已绑定至具体续跳 idem 键，推进中。
	LeaseBound LeaseStatus = "bound"
	// LeaseDone 续跳已推进完成，残留案将转 closed。
	LeaseDone LeaseStatus = "done"
	// LeaseExpired 令牌超过 TTL 未使用，自动失效，需重新申请。
	LeaseExpired LeaseStatus = "expired"
)

// ─────────────────────────────────────────────────────────────────────────────
// BatchResidualCase — 批量 hop 部分失败残留案
// ─────────────────────────────────────────────────────────────────────────────
// BatchResidualCase 记录一次 BatchHop 在第 FailIndex 项失败时的残留快照。
// 核心不变量：
// - SucceededHops 与 BeforeRoot/PartialRoot 须来自同一次 BatchHop 调用；
// - FailedReason 为原始错误字符串，不解析；
// - ProofID 非空时 Status 至少为 proven；
// - LeaseID 非空时 Status 至少为 resuming。
type BatchResidualCase struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Generation int64 `json:"generation"`
	TotalItems int `json:"total_items"`
	FailIndex  int `json:"fail_index"`
	SucceededHops []HopSummary `json:"succeeded_hops"`
	FailedReason  string       `json:"failed_reason"`
	BeforeRoot string `json:"before_root"`
	PartialRoot string         `json:"partial_root"`
	Status      ResidualStatus `json:"status"`
	ProofID     string         `json:"proof_id,omitempty"`
	LeaseID    string `json:"lease_id,omitempty"`
	Confirm1By string `json:"confirm1_by,omitempty"`
	Confirm2By string     `json:"confirm2_by,omitempty"`
	Confirm1At *time.Time `json:"confirm1_at,omitempty"`
	Confirm2At *time.Time `json:"confirm2_at,omitempty"`
	Note       string     `json:"note,omitempty"`
	DetectedAt time.Time  `json:"detected_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// HopSummary — 已成功单笔 hop 的摘要
// ─────────────────────────────────────────────────────────────────────────────
// HopSummary 记录 BatchHop 中已执行成功的一笔 hop 信息，供证明与续跳幂等去重使用。
type HopSummary struct {
	Index int    `json:"index"`
	From  string `json:"from"`
	To    string `json:"to"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Idem   string `json:"idem,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ResumeLease — 续跳互斥世代租约
// ─────────────────────────────────────────────────────────────────────────────
// ResumeLease 是一次续跳操作的互斥世代令牌。
// 核心约束：
// - 同一 residual_case_id 同一时刻至多一条 open/bound 租约；
// - Generation 须与 BatchGeneration.generation 匹配，失配视为过期；
// - ResumeFrom 指定从第几项开始续跳（必须 >= FailIndex）；
// - BoundIdem 非空时 Status 为 bound；重复请求同一 BoundIdem 返回幂等结果。

type ResumeLease struct {
	ID             string `json:"id"`
	ResidualCaseID string `json:"residual_case_id"`
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	Generation int64  `json:"generation"`
	ResumeFrom int `json:"resume_from"`
	BoundIdem string      `json:"bound_idem,omitempty"`
	Status    LeaseStatus `json:"status"`
	ExpiresAt time.Time   `json:"expires_at"`
	ConfirmedBy string     `json:"confirmed_by,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	Note string `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// HopOccupyProof — 部分成功 hop 资金占用证明
// ─────────────────────────────────────────────────────────────────────────────
// HopOccupyProof 记录 BatchHop 部分成功子集的科目合计与守恒证明。
// 证明构造：
// Digest = sha256(BeforeRoot + "|" + PartialRoot + "|" + residual_case_id)
// 验证时重计算并；Digest 不一致 → ErrProofDigestMismatch。
// SubjectTotals 列出已成功子集中每个科目的资金变动量；
// ConserveOK 若为 true 表示已成功 hops 的借贷两端合计守恒。

type HopOccupyProof struct {
	ID string `json:"id"`
	ResidualCaseID string `json:"residual_case_id"`
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	BeforeRoot string `json:"before_root"`
	PartialRoot string `json:"partial_root"`
	SubjectTotals []BookTotal `json:"subject_totals"`
	ConserveOK bool      `json:"conserve_ok"`
	Digest     string    `json:"digest"`
	BuiltBy    string    `json:"built_by,omitempty"`
	VerifiedAt time.Time `json:"verified_at"`
	CreatedAt time.Time `json:"created_at"`
}

// BookTotal 记录某一科目（book）在部分成功 hop 中的资金变动合计。
type BookTotal struct {
	Book string `json:"book"`
	Asset string `json:"asset"`
	Delta string `json:"delta"`
	After string `json:"after"`
}

// ─────────────────────────────────────────────────────────────────────────────
// BatchGeneration — 用户资产批量 hop CAS 世代计数器
// ─────────────────────────────────────────────────────────────────────────────
// BatchGeneration 追踪用户对特定资产发起的 BatchHop 会话世代。
// CAS 协议（SQL 层原子 UPDATE）：
// Acquire（开启新 BatchHop）:
// UPDATE portfolio_batch_generations
// SET generation = generation + 1,
// status = 'active',
// updated_at = NOW(3)
// WHERE user_id = ? AND asset = ? AND generation = ?
// -- 影响行数 = 0 → ErrStaleGeneration；= 1 → 成功，新世代 = old+1
// Release（BatchHop 完成或失败收口后）:
// UPDATE portfolio_batch_generations
// SET status = 'idle', updated_at = NOW(3)
// WHERE user_id = ? AND asset = ? AND generation = ?
// 存在 status='active' 记录时：同用户同资产的新 BatchHop 被拒绝（ErrConcurrentBatch）。

type BatchGeneration struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Generation int64     `json:"generation"`
	Status     string    `json:"status"` // idle | active
	UpdatedAt  time.Time `json:"updated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ResidualKind — 残留案发现来源
// ─────────────────────────────────────────────────────────────────────────────
// ResidualKind 描述残留案的发现来源，供 worker 分类扫描。

type ResidualKind string

const (
	// ResidualKindBatchFail item i 执行失败直接触发。
	ResidualKindBatchFail ResidualKind = "batch_fail"
	// ResidualKindScanDiscover worker 巡检发现无残留案的孤儿 hop 流水。
	ResidualKindScanDiscover ResidualKind = "scan_discover"
)

// ─────────────────────────────────────────────────────────────────────────────
// OpenResidualKey — 并行互斥查找键
// ─────────────────────────────────────────────────────────────────────────────
// OpenResidualKey 用于按 user_id + asset 快速查找未闭合残留案。

type OpenResidualKey struct {
	UserID string
	Asset string
}

// ─────────────────────────────────────────────────────────────────────────────
// ResumeRequest — 续跳请求参数
// ─────────────────────────────────────────────────────────────────────────────
// ResumeRequest 描述一次续跳操作的参数。
// LeaseID 必须为有效的未过期 ResumeLease；
// ResumeItems 为从 FailIndex 开始的续跳 BatchItem 列表，不得包含已成功索引；
// ConfirmedBy 为操作人 ID（双人确认场景）。

type ResumeRequest struct {
	ResidualCaseID string `json:"residual_case_id"`
	LeaseID        string `json:"lease_id"`
	UserID         string `json:"user_id"`
	Asset string `json:"asset"`
	Generation int64 `json:"generation"`
	ResumeItems []ResumeItem `json:"resume_items"`
	ConfirmedBy string       `json:"confirmed_by,omitempty"`
	Note        string       `json:"note,omitempty"`
}

// ResumeItem 描述续跳中一笔待执行的 hop。Index 须 >= BatchResidualCase.FailIndex。
type ResumeItem struct {
	Index int `json:"index"`
	From string `json:"from"`
	To string `json:"to"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Idem   string `json:"idem"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ResumeResult — 续跳结果
// ─────────────────────────────────────────────────────────────────────────────
// ResumeResult 描述一次续跳操作的执行结果。
type ResumeResult struct {
	ResidualCaseID string `json:"residual_case_id"`
	LeaseID string `json:"lease_id"`
	Executed []HopSummary `json:"executed"`
	Skipped  []HopSummary `json:"skipped"`
	NewStatus ResidualStatus `json:"new_status"`
}

// ─────────────────────────────────────────────────────────────────────────────
// GenToken — 世代令牌
// ─────────────────────────────────────────────────────────────────────────────
// GenToken 由 BatchGeneration 引擎颁发，携带 UserID/Asset/Generation，供续跳调用方鉴权。
type GenToken struct {
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	Generation int64  `json:"generation"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Valid 检查令牌是否未过期。

   func (t GenToken) Valid(now time.Time) bool {
	  return now.Before(t.ExpiresAt)
   }
