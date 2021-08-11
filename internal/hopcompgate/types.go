package hopcompgate

import (
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Case 状态机
// ──────────────────────────────────────────────────────────────────────────────
// CaseState 补偿仓状态。
type CaseState string

const (
	// CaseOpen 补偿仓已建立，等待执行 CompensateMove。
	CaseOpen CaseState = "open"
	// CaseCompensating 补偿 Move 已提交，等待确认。
	CaseCompensating CaseState = "compensating"
	// CaseClosed 补偿 Move 已成功，资金已归位，仓已关闭。
	CaseClosed CaseState = "closed"
	// CaseBlocked 补偿 Move 持续失败或世代过期，需人工介入。
	CaseBlocked CaseState = "blocked"
)

// CaseTransition 合法的状态迁移表。
var CaseTransition = map[CaseState][]CaseState{
	CaseOpen:         {CaseCompensating, CaseBlocked},
	CaseCompensating: {CaseClosed, CaseBlocked, CaseOpen},
	CaseBlocked:      {CaseOpen},
}

// CanTransition 判断状态迁移是否合法。

   func (s CaseState) CanTransition(next CaseState) bool {
	   for _, allowed := range CaseTransition[s] {
		   if allowed == next {
			   return true
		   }
	   }
	  return false
  }

// ──────────────────────────────────────────────────────────────────────────────
// Lease 状态机
// ──────────────────────────────────────────────────────────────────────────────
// LeaseState 补偿世代租约状态。

type LeaseState string

const (
	// LeaseOpen 租约已建立，未绑定执行令牌。
	LeaseOpen LeaseState = "open"
	// LeaseBound 已绑定，补偿 Move 正在执行。
	LeaseBound LeaseState = "bound"
	// LeaseDone 补偿已完成，租约关闭。
	LeaseDone LeaseState = "done"
	// LeaseExpired 超时未完成，租约失效，须重新申请。
	LeaseExpired LeaseState = "expired"
)

// LeaseTransition 合法的租约迁移表。
var LeaseTransition = map[LeaseState][]LeaseState{
	LeaseOpen: {LeaseBound, LeaseExpired},
	LeaseBound: {LeaseDone,
		LeaseExpired, LeaseOpen},
	LeaseExpired: {},
	LeaseDone:    {},
}

// CanTransition 判断租约迁移是否合法。
  func (s LeaseState) CanTransition(next LeaseState) bool {
	  for _, allowed := range LeaseTransition[s] {
		   if allowed == next {
			  return true
		   }
	   }
	   return false
   }

// ──────────────────────────────────────────────────────────────────────────────
// HopCompCase
// ──────────────────────────────────────────────────────────────────────────────
// HopCompCase 同片 hop 绑定失败补偿仓。
// 每次 BindTransfer 失败后必须新建或合并入已有的同 idem Case。
type HopCompCase struct {
	// ID 补偿仓全局唯一 ID，也作 hop_comp_id 供他包只读。
	ID string `json:"id"`
	// HopIdem hop 请求的幂等键（来源 HopReq.IdempotencyKey）。
	HopIdem string `json:"hop_idem"`
	// UserID 所属用户。
	UserID string `json:"user_id"`
	// Asset 资产代码（大写）。
	Asset string `json:"asset"`
	// Amount 需补偿金额（字符串 decimal）。
	Amount string `json:"amount"`
	// FromBook 原来的 from 科目（Move 的 source）。
	FromBook string `json:"from_book"`
	// ToBook 原来的 to 科目（Move 的 dest；补偿时反向）。
	ToBook string `json:"to_book"`
	// MoveID 成功完成的 Move 交易 ID。
	MoveID string `json:"move_id"`
	// BindErr BindTransfer 失败的原因摘要（≤512 字符）。
	BindErr string `json:"bind_err"`
	// State 当前补偿仓状态。
	State CaseState `json:"state"`
	// Generation 当前世代（单调自增；每次重试 +1）。
	Generation int64 `json:"generation"`
	// SameShard 只能为 true；false 时本仓不得创建。
	SameShard bool `json:"same_shard"`
	// Actor 触发者（系统/操作员）。
	Actor string `json:"actor,omitempty"`
	// Region 持久化 region。
	Region string `json:"region,omitempty"`
	// RetryCount 自动重试次数。
	RetryCount int `json:"retry_count"`
	// ForcedBy 双人强制补偿操作员 ID（空=未强制）。
	ForcedBy string `json:"forced_by,omitempty"`
	// ForceReason 双人强制补偿原因。
	ForceReason string `json:"force_reason,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最近更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// ClosedAt 关闭时间（nil=未关闭）。
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// HopCompID 返回 hop_comp_id（供外部包只读引用）。
  func (c *HopCompCase) HopCompID() string {
	  if c == nil {
		   return ""
	  }
	  return c.ID
   }

// ──────────────────────────────────────────────────────────────────────────────
// CompensateLease
// ──────────────────────────────────────────────────────────────────────────────
// CompensateLease 补偿世代租约。
// CompensateMove 必须持有未过期且 State=bound 的 Lease 才能执行。

type CompensateLease struct {
	// ID 租约唯一 ID。
	ID string `json:"id"`
	// CaseID 关联的 HopCompCase.ID。
	CaseID string `json:"case_id"`
	// Generation 世代号（与 HopCompCase.Generation 严格一致）。
	Generation int64 `json:"generation"`
	// Token 幂等执行令牌（由 hopcompgate 内部分配）。
	Token string `json:"token"`
	// State 租约当前状态。
	State LeaseState `json:"state"`
	// ExpiresAt 到期时间；补偿必须在此之前完成。
	ExpiresAt time.Time `json:"expires_at"`
	// BoundAt 绑定时间。
	BoundAt *time.Time `json:"bound_at,omitempty"`
	// DoneAt 完成时间。
	DoneAt *time.Time `json:"done_at,omitempty"`
	// ExpiredAt 吊销时间。
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
}

// Valid 检查租约是否有效（未过期且非 done/expired）。
   func (l *CompensateLease) Valid(now time.Time) bool {
	   if l == nil {
		  return false
	  }
	   if l.State == LeaseDone || l.State == LeaseExpired {
		  return false
	   }
	   return now.Before(l.ExpiresAt)
  }

// ──────────────────────────────────────────────────────────────────────────────
// DoubleMoveProof
// ──────────────────────────────────────────────────────────────────────────────
// DoubleMoveKind 双移证明类型。
type DoubleMoveKind string

const (
	// ProofDoubleMove Move 成功且补偿也执行了一次有效 Move（双向 Move），导致双移。
	ProofDoubleMove DoubleMoveKind = "double_move"
	// ProofMissedCompensate Move 成功但补偿从未执行，世代过期后资金仍在错误科目。
	ProofMissedCompensate DoubleMoveKind = "missed_compensate"
	// ProofReentryWhileCompensating 补偿未关闭时同 idem Hop 重入。
	ProofReentryWhileCompensating DoubleMoveKind = "reentry_while_compensating"
)

// DoubleMoveProof 双移/漏补证明。
// 由 double-move 巡检 worker 在检测到异常时写入；只追加不修改。

type DoubleMoveProof struct {
	// ID 证明唯一 ID。
	ID string `json:"id"`
	// CaseID 关联的 HopCompCase.ID。
	CaseID string `json:"case_id"`
	// Kind 证明类型。
	Kind DoubleMoveKind `json:"kind"`
	// HopIdem 原始 hop 幂等键。
	HopIdem string `json:"hop_idem"`
	// UserID 所属用户。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Amount 涉及金额。
	Amount string `json:"amount"`
	// MoveID 第一次 Move 交易 ID。
	MoveID string `json:"move_id"`
	// CompensateMoveID 补偿 Move 交易 ID（ProofMissedCompensate 时为空）。
	CompensateMoveID string `json:"compensate_move_id,omitempty"`
	// SecondMoveID 第二次 Move 交易 ID（ProofDoubleMove 时填充）。
	SecondMoveID string `json:"second_move_id,omitempty"`
	// Evidence 人类可读的证明摘要（≤2048 字符）。
	Evidence string `json:"evidence"`
	// DetectedAt 发现时间。
	DetectedAt time.Time `json:"detected_at"`
	// ExportedAt 最近一次导出时间（nil=未导出）。
	ExportedAt *time.Time `json:"exported_at,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// 请求/响应类型
// ──────────────────────────────────────────────────────────────────────────────
// OpenCaseReq 打开新补偿仓的请求（由 fundpath 在 BindTransfer 失败时发起）。
type OpenCaseReq struct {
	HopIdem string `json:"hop_idem"`
	UserID string `json:"user_id"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	FromBook string `json:"from_book"`
	ToBook   string `json:"to_book"`
	MoveID   string `json:"move_id"`
	BindErr string `json:"bind_err"`
	// SameShard 必须为 true；跨片调用方不得传 true。
	SameShard bool   `json:"same_shard"`
	Actor     string `json:"actor,omitempty"`
	Region string `json:"region,omitempty"`
}

// AcquireLeaseReq 申请补偿世代租约。
type AcquireLeaseReq struct {
	CaseID string `json:"case_id"`
	TTL int64 `json:"ttl_seconds"` // 默认 300s；最大 1800s。
}

// CompensateReq 执行补偿的请求。

type CompensateReq struct {
	CaseID string `json:"case_id"`
	LeaseID string `json:"lease_id"`
	Token   string `json:"token"`
	// Generation 客户端持有的世代；与 Lease.Generation 不符则拒。
	Generation int64 `json:"generation"`
}

// ForceCompensateReq 双人确认强制补偿。

type ForceCompensateReq struct {
	CaseID   string `json:"case_id"`
	Actor    string `json:"actor"`
	Approver string `json:"approver"`
	Reason   string `json:"reason"`
}

// CaseQueryReq 补偿仓查询过滤器。

type CaseQueryReq struct {
	UserID string    `json:"user_id,omitempty"`
	State  CaseState `json:"state,omitempty"`
	After   time.Time `json:"after,omitempty"`
	Before  time.Time `json:"before,omitempty"`
	HopIdem string    `json:"hop_idem,omitempty"`
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// ProofExportReq 证明导出过滤器。

type ProofExportReq struct {
	CaseID string         `json:"case_id,omitempty"`
	Kind   DoubleMoveKind `json:"kind,omitempty"`
	After  time.Time      `json:"after,omitempty"`
	Limit  int            `json:"limit,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// 常量
// ──────────────────────────────────────────────────────────────────────────────

const (
	// DefaultLeaseTTL 默认补偿世代租约时长（秒）。
	DefaultLeaseTTL = int64(300)
	// MaxLeaseTTL 最大租约时长（秒）。
	MaxLeaseTTL = int64(1800)
	// MaxRetryCount 自动重试上限；超过则转 blocked。
	MaxRetryCount = 5
	// TableHopComps fundpath_hop_comps 表名。
	TableHopComps = "fundpath_hop_comps"
	// TableCompLeases fundpath_comp_leases 表名。
	TableCompLeases = "fundpath_comp_leases"
	// TableDoubleMove fundpath_double_move_proofs 表名。
	TableDoubleMove = "fundpath_double_move_proofs"
)
