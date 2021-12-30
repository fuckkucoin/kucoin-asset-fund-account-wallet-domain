package wdtriplegate

import (
	"fmt"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// TripleLease — 三联租约（ticket_clear ⊕ kyt_pin ⊕ list_gen ⊕ policy_ver）
// ──────────────────────────────────────────────────────────────────────────────
// LeaseState 租约生命周期状态机。
// 合法转换：Issued → Bound → Consumed/Expired/Blocked。
// Issued: 预检通过后立即签发，持有快照世代。
// Bound: 广播前复验成功，绑定当前世代未漂移。
// Consumed: 广播成功后收口，租约终态。
// Expired: TTL 到期未绑定，需重跑 CheckTriple。
// Blocked: 漂移检测发现偏移，挡写终态。
type LeaseState string

const (
	LeaseIssued LeaseState = "issued" // 已签发，等待广播前复验绑定
	LeaseBound    LeaseState = "bound"    // 广播前复验通过，已绑定
	LeaseConsumed LeaseState = "consumed" // 广播成功，已消费
	LeaseExpired  LeaseState = "expired"  // TTL 超期，需重检
	LeaseBlocked LeaseState = "blocked" // 漂移证明落仓，挡写
)

// TripleLease 三联闸预检后签发的世代租约。
// 快照：ticket_clear（无未结异常单）⊕ kyt_pin（KYT 决策）⊕ list_gen（白名单世代）⊕ policy_ver（策略代次）。
// 广播前必须复验 list_gen / policy_ver 未漂移，否则拒绝。

type TripleLease struct {
	TripleLeaseID string `json:"triple_lease_id"`
	MovementID    string `json:"movement_id"`
	UserID        string `json:"user_id"`
	Chain string `json:"chain"`
	Dest string `json:"dest"`
	// 三联快照
	TicketClear bool   `json:"ticket_clear"` // 签发时无未结异常单
	KYTPin      string `json:"kyt_pin"`      // KYT 决策快照（"allow"）
	ListGen int64 `json:"list_gen"` // destallow 白名单世代快照
	PolicyVer int `json:"policy_ver"` // policypack 策略代次快照
	State    LeaseState `json:"state"`
	IssuedAt time.Time  `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"` // TTL 截止；过期后需重检
	BoundAt   time.Time `json:"bound_at,omitempty"`
	ConsumedAt time.Time `json:"consumed_at,omitempty"`
	ExpiredAt  time.Time `json:"expired_at,omitempty"`
	BlockedAt time.Time `json:"blocked_at,omitempty"`
	DriftCaseID string `json:"drift_case_id,omitempty"` // 关联漂移案件（Blocked 时写入）
	BlockReason string `json:"block_reason,omitempty"`
	IssuedBy string `json:"issued_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTerminal 租约是否处于终态。
  func (l *TripleLease) IsTerminal() bool {
	  if l == nil {
		   return false
	   }
	   return l.State == LeaseConsumed || l.State == LeaseExpired || l.State == LeaseBlocked
  }

// IsActive 租约是否有效（Issued 或 Bound 且未超期）。

  func (l *TripleLease) IsActive(now time.Time) bool {
	   if l == nil {
		   return false
	  }
	   return (l.State == LeaseIssued || l.State == LeaseBound) && now.Before(l.ExpiresAt)
   }

// ──────────────────────────────────────────────────────────────────────────────
// PreflightBind — 广播前世代复验绑定记录
// ──────────────────────────────────────────────────────────────────────────────
// PreflightBind 广播前世代复验结果。
// 复验时重新读取 list_gen / policy_ver，与租约快照。
// 任一不匹配则不允许广播；产出 TripleDriftCase。

type PreflightBind struct {
	BindID        string `json:"bind_id"`
	MovementID    string `json:"movement_id"`
	TripleLeaseID string `json:"triple_lease_id"`
	// 复验时读取的世代（与租约快照核对）
	ListGenAtBind int64 `json:"list_gen_at_bind"`
	PolicyVerAtBind int `json:"policy_ver_at_bind"`
	// 复验结论
	BroadcastAllowed bool `json:"broadcast_allowed"`
	RefuseReason string `json:"refuse_reason,omitempty"`
	DriftDetected bool `json:"drift_detected,omitempty"`
	BoundAt time.Time `json:"bound_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// TripleDriftCase — 三联漂移证明案件
// ──────────────────────────────────────────────────────────────────────────────
// DriftKind 漂移类型。
type DriftKind string

const (
	DriftListGen DriftKind = "list_gen" // 白名单世代漂移
	DriftPolicyVer DriftKind = "policy_ver" // 策略代次漂移
	DriftKYTPin    DriftKind = "kyt_pin"    // KYT 钉撤销漂移
)

// DriftCaseState 漂移案件状态机。
// 合法转换：Open → Proven → Closed。
// Open: 已发现漂移，movement 被挡写。
// Proven: 证明摘要已附上，等待双人确认。
// Closed: 双人确认后关闭（终态）；重检后方可重新放行。
type DriftCaseState string

const (
	DriftOpen   DriftCaseState = "open"   // 已发现漂移
	DriftProven DriftCaseState = "proven" // 已附证明，等待确认
	DriftClosed DriftCaseState = "closed" // 已关闭（终态）
)

// TripleDriftCase 三联漂移案件。
// 由 Triple Drift Detector 在广播前复验发现世代变化时产出。
// proof_checksum = SHA-256(movement_id|drift_kind|lease_gen|current_gen|detected_at)。
// 关联 TripleLease 进入 Blocked；后续放行必须重检或经四眼收口。
type TripleDriftCase struct {
	DriftCaseID string `json:"drift_case_id"`
	MovementID  string `json:"movement_id"`
	TripleLeaseID string    `json:"triple_lease_id"`
	DriftKind     DriftKind `json:"drift_kind"`
	// 世代/代次核对
	LeaseListGen   int64 `json:"lease_list_gen"`   // 租约签发时快照
	CurrentListGen int64 `json:"current_list_gen"` // 漂移检测时读取
	LeasePolicyVer int   `json:"lease_policy_ver"` // 租约签发时快照
	CurrentPolicyVer int    `json:"current_policy_ver"` // 漂移检测时读取
	LeaseKYTPin      string `json:"lease_kyt_pin"`      // 租约签发时快照
	CurrentKYTPin    string `json:"current_kyt_pin"`    // 漂移检测时读取
	State DriftCaseState `json:"state"`
	ProofChecksum string `json:"proof_checksum,omitempty"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	CoConfirmedBy string `json:"co_confirmed_by,omitempty"`
	CloseNote string `json:"close_note,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	ProvenAt  time.Time `json:"proven_at,omitempty"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTerminal 案件是否已终态。

  func (d *TripleDriftCase) IsTerminal() bool {
	  if d == nil {
		   return false
	  }
	  return d.State == DriftClosed
   }

// ──────────────────────────────────────────────────────────────────────────────
// 请求/响应结构
// ──────────────────────────────────────────────────────────────────────────────
// IssueLeaseReq 签发三联租约请求（Preflight 成功后调用）。

type IssueLeaseReq struct {
	MovementID string `json:"movement_id"`
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Dest  string `json:"dest"`
	TicketClear bool   `json:"ticket_clear"`
	KYTPin      string `json:"kyt_pin"`
	ListGen     int64  `json:"list_gen"`
	PolicyVer   int    `json:"policy_ver"`
	IssuedBy    string `json:"issued_by"`
	TTLSeconds int `json:"ttl_seconds,omitempty"` // 默认 300s
}

// BindReq 广播前世代复验请求。
type BindReq struct {
	MovementID string `json:"movement_id"`
	TripleLeaseID    string `json:"triple_lease_id"`
	CurrentListGen   int64  `json:"current_list_gen"`
	CurrentPolicyVer int    `json:"current_policy_ver"`
	CurrentKYTPin string `json:"current_kyt_pin"`
}

// BindResult 广播前世代复验结果。

type BindResult struct {
	BindID           string `json:"bind_id"`
	BroadcastAllowed bool   `json:"broadcast_allowed"`
	DriftDetected    bool   `json:"drift_detected"`
	DriftCaseID      string `json:"drift_case_id,omitempty"`
	RefuseReason string `json:"refuse_reason,omitempty"`
}

// ConfirmDriftCaseReq 四眼确认关闭漂移案件。
type ConfirmDriftCaseReq struct {
	DriftCaseID string `json:"drift_case_id"`
	ConfirmedBy   string `json:"confirmed_by"`
	CoConfirmedBy string `json:"co_confirmed_by"`
	Note string `json:"note,omitempty"`
}

// LeaseStatusResp 租约状态查询响应。
type LeaseStatusResp struct {
	Lease *TripleLease   `json:"lease"`
	Bind  *PreflightBind `json:"bind,omitempty"`
	DriftCases []*TripleDriftCase `json:"drift_cases,omitempty"`
}

// GenMismatchErr 世代不匹配错误（三联闸专用）。

type GenMismatchErr struct {
	MovementID string
	Kind     DriftKind
	Expected int64
	Got      int64
}

   func (e *GenMismatchErr) Error() string {
	   return fmt.Sprintf("wdtriplegate: gen mismatch movement=%s kind=%s expected=%d got=%d",
		   e.MovementID, e.Kind, e.Expected, e.Got)
  }

// CopyLease 浅拷贝防止外部修改内部状态。
  func CopyLease(l TripleLease) TripleLease { return l }

// CopyDriftCase 浅拷贝。

  func CopyDriftCase(d TripleDriftCase) TripleDriftCase { return d }

// CopyBind 浅拷贝。
   func CopyBind(b PreflightBind) PreflightBind { return b }
