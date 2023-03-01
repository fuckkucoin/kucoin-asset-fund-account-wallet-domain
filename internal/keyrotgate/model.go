package keyrotgate

import (
	"fmt"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// RotateLease — 轮换世代租约
// ──────────────────────────────────────────────────────────────────────────────
// LeaseState 轮换租约生命周期状态机。
// 合法转换：Open → Approved → Executing → Done/Blocked。
// Open: OpenRotate 抬升 rotate_gen 后立即签发。
// Approved: 第二人批准；进入可执行态。
// Executing: ExecuteRotate 锁定，互斥在途签名。
// Done: ExecuteRotate 成功完成，指纹证明已落仓。
// Blocked: 存在未结 SignBind 或漂移检测拒绝，进入阻塞待队列。
type LeaseState string

const (
	LeaseOpen LeaseState = "open" // 已签发，等待第二人批准
	LeaseApproved  LeaseState = "approved"  // 已批准，可进入执行
	LeaseExecuting LeaseState = "executing" // 执行中，互斥在途签名
	LeaseDone      LeaseState = "done"      // 执行完毕，终态
	LeaseBlocked LeaseState = "blocked" // 被在途签名或漂移阻塞，终态
)

// RotateLease HSM 密钥槽轮换世代租约。
// 记录源槽/目标槽指纹快照、rotate_gen 世代号与租约有效期。
// ExecuteRotate 必须持有 Approved 且未过期的租约方可执行。

type RotateLease struct {
	RotateLeaseID string `json:"rotate_lease_id"`
	TicketID  string `json:"ticket_id"`   // keycer Ticket.ID（rotate 工单）
	SrcSlotID string `json:"src_slot_id"` // 源槽 ID
	DstSlotID string `json:"dst_slot_id"` // 目标槽 ID
	SrcFingerprint string `json:"src_fingerprint"` // 签发时源槽指纹快照
	DstFingerprint string `json:"dst_fingerprint"` // 签发时目标槽指纹快照（空表示 dst 为 Empty）
	RotateGen      int64  `json:"rotate_gen"`      // 本次轮换世代号（单调递增）
	State    LeaseState `json:"state"`
	OpenedBy string     `json:"opened_by"`
	ApprovedBy string `json:"approved_by,omitempty"`
	ExecutedBy string `json:"executed_by,omitempty"`
	Note       string `json:"note,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	ExpiresAt  time.Time  `json:"expires_at"` // TTL 截止
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	ExecutingAt *time.Time `json:"executing_at,omitempty"`
	DoneAt      *time.Time `json:"done_at,omitempty"`
	BlockedAt *time.Time `json:"blocked_at,omitempty"`
	BlockReason string `json:"block_reason,omitempty"` // Blocked 时填写原因
	ProofID     string `json:"proof_id,omitempty"`     // Done 时关联 FingerprintProof.ID
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTerminal 租约是否处于终态。
   func (l *RotateLease) IsTerminal() bool {
	  if l == nil {
		   return false
	   }
	   return l.State == LeaseDone || l.State == LeaseBlocked
  }

// IsActive 租约是否有效（未超期且非终态）。
   func (l *RotateLease) IsActive(now time.Time) bool {
	  if l == nil {
		  return false
	   }
	  return !l.IsTerminal() && now.Before(l.ExpiresAt)
   }

// String 实现 fmt.Stringer，用于日志输出。
  func (l *RotateLease) String() string {
	   if l == nil {
		  return "<nil RotateLease>"
	   }
	   return fmt.Sprintf("RotateLease{id=%s src=%s dst=%s gen=%d state=%s}",
		  l.RotateLeaseID, l.SrcSlotID, l.DstSlotID, l.RotateGen, l.State)
  }

// ──────────────────────────────────────────────────────────────────────────────
// SignBind — 在途签名/解锁绑定记录
// ──────────────────────────────────────────────────────────────────────────────
// SignBindState 在途签名绑定状态。
// 合法转换：Active → Released/Expired/Rejected。
type SignBindState string

const (
	SignBindActive   SignBindState = "active"   // 在途，持有源槽指纹引用
	SignBindReleased SignBindState = "released" // 签名/解锁完成，已释放
	SignBindExpired  SignBindState = "expired"  // TTL 超期自动释放
	SignBindRejected SignBindState = "rejected" // 源槽已 Retired，旧指纹拒签
)

// SignBind 在途签名或 Unlock 操作对源槽指纹的绑定记录。
// ExecuteRotate 检测到有效 SignBind 时拒绝执行（或进入 Blocked 队列）。
// 旧指纹检测到槽已 Retired 时产出 KeyRotDriftCase。
type SignBind struct {
	SignBindID  string `json:"sign_bind_id"`
	SrcSlotID   string `json:"src_slot_id"`  // 绑定的源槽 ID
	Fingerprint string `json:"fingerprint"`  // 绑定时的槽指纹
	OperationID string `json:"operation_id"` // 签名/解锁操作 ID（外部引用）
	OperationKind string `json:"operation_kind"` // "sign" 或 "unlock"
	BoundBy       string `json:"bound_by"`       // 发起操作的 Actor
	State SignBindState `json:"sign_bind_state"`
	BoundAt    time.Time  `json:"bound_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ExpiredAt  *time.Time `json:"expired_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	DriftCaseID string `json:"drift_case_id,omitempty"` // 产出漂移案件时关联
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsActive SignBind 是否仍在途（持有指纹引用）。

  func (b *SignBind) IsActive(now time.Time) bool {
	   if b == nil {
		  return false
	  }
	   return b.State == SignBindActive && now.Before(b.ExpiresAt)
  }

// ──────────────────────────────────────────────────────────────────────────────
// KeyRotDriftCase — 指纹漂移案件
// ──────────────────────────────────────────────────────────────────────────────
// DriftCaseState 漂移案件状态机。
// 合法转换：Open → Proven → Closed。
type DriftCaseState string

const (
	DriftOpen DriftCaseState = "open" // 检测到漂移，等待双人确认
	DriftProven DriftCaseState = "proven" // 证明已提交，等待关闭
	DriftClosed DriftCaseState = "closed" // 已关闭，终态
)

// KeyRotDriftCase 指纹漂移案件：检测到使用旧指纹签名而源槽已 Retired 时产出。
// 证明通过后方可关闭；关闭前阻止后续 Unlock/Rotate。
type KeyRotDriftCase struct {
	DriftCaseID string `json:"drift_case_id"`
	SrcSlotID      string `json:"src_slot_id"`     // 已退役的源槽
	OldFingerprint string `json:"old_fingerprint"` // 旧指纹（已 Retired 槽）
	NewFingerprint string `json:"new_fingerprint"` // 新槽的指纹（可为空）
	SignBindID string `json:"sign_bind_id"` // 触发漂移的 SignBind
	RotateLeaseID string         `json:"rotate_lease_id"` // 关联的轮换租约（可为空）
	State         DriftCaseState `json:"state"`
	DriftDetectedAt time.Time  `json:"drift_detected_at"`
	ProvenAt        *time.Time `json:"proven_at,omitempty"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
	ProofChecksum string `json:"proof_checksum,omitempty"`  // 双人确认后写入；唯一
	ConfirmedBy   string `json:"confirmed_by,omitempty"`    // 第一确认人
	CoConfirmedBy string `json:"co_confirmed_by,omitempty"` // 第二确认人（四眼）
	Note          string `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTerminal 漂移案件是否已终态。
   func (d *KeyRotDriftCase) IsTerminal() bool {
	  if d == nil {
		   return false
	  }
	  return d.State == DriftClosed
  }

// ──────────────────────────────────────────────────────────────────────────────
// FingerprintProof — 轮换前后指纹证明
// ──────────────────────────────────────────────────────────────────────────────
// FingerprintProof 轮换完成后落仓的指纹前后证明。
// 证明缺失时阻止后续 Unlock/Rotate（通过 Gate.RequireProof 校验）。

type FingerprintProof struct {
	ProofID string `json:"proof_id"`
	RotateLeaseID string `json:"rotate_lease_id"` // 关联轮换租约
	TicketID      string `json:"ticket_id"`       // keycer rotate 工单
	SrcSlotID     string `json:"src_slot_id"`
	DstSlotID     string `json:"dst_slot_id"`
	BeforeFingerprint string `json:"before_fingerprint"` // 源槽退役前指纹
	AfterFingerprint  string `json:"after_fingerprint"`  // 目标槽加载后指纹
	RotateGen         int64  `json:"rotate_gen"`
	ProofChecksum string `json:"proof_checksum"` // SHA-256(before||after||rotate_gen||ticket_id)
	IssuedBy      string `json:"issued_by"`
	IssuedAt time.Time `json:"issued_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Request/Response 类型
// ──────────────────────────────────────────────────────────────────────────────
// OpenLeaseReq OpenRotate 后签发轮换租约的请求。

type OpenLeaseReq struct {
	TicketID       string `json:"ticket_id"`
	SrcSlotID      string `json:"src_slot_id"`
	DstSlotID      string `json:"dst_slot_id"`
	SrcFingerprint string `json:"src_fingerprint"`
	DstFingerprint string `json:"dst_fingerprint,omitempty"`
	OpenedBy string `json:"opened_by"`
	Note       string `json:"note,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"` // 0 使用默认值
}

// ApproveLeaseReq 批准轮换租约请求（第二人）。
type ApproveLeaseReq struct {
	RotateLeaseID string `json:"rotate_lease_id"`
	ApprovedBy    string `json:"approved_by"`
}

// ExecuteLeaseReq 执行轮换的请求。
type ExecuteLeaseReq struct {
	RotateLeaseID string `json:"rotate_lease_id"`
	ExecutedBy string `json:"executed_by"`
}

// BindSignReq 绑定在途签名/解锁的请求。
type BindSignReq struct {
	SrcSlotID     string `json:"src_slot_id"`
	Fingerprint   string `json:"fingerprint"`
	OperationID   string `json:"operation_id"`
	OperationKind string `json:"operation_kind"` // "sign" | "unlock"
	BoundBy string `json:"bound_by"`
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

// ReleaseSignReq 释放在途签名绑定请求。
type ReleaseSignReq struct {
	SignBindID string `json:"sign_bind_id"`
}

// ProveRotateReq 提交漂移案件证明请求。
type ProveRotateReq struct {
	DriftCaseID   string `json:"drift_case_id"`
	ProofChecksum string `json:"proof_checksum"`
	ConfirmedBy string `json:"confirmed_by"`
	CoConfirmedBy string `json:"co_confirmed_by"`
	Note string `json:"note,omitempty"`
}

// CloseDriftCaseReq 关闭漂移案件请求。

type CloseDriftCaseReq struct {
	DriftCaseID string `json:"drift_case_id"`
	ClosedBy    string `json:"closed_by"`
}
