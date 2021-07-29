package invleasegate

import "time"

// ─── 租约状态机 ────────────────────────────────────────────────────────────────
// LeaseState 预检租约状态机枚举。
// 状态迁移：
// issued → bound （Check 放行，提交路径绑定租约）
// bound → committed （提交复验通过，出金/归集路径放行）
// issued → expired （TTL 超时）
// bound → expired （TTL 超时）
// issued → blocked （AdviceFlip：issued 期间建议翻转）
// bound → blocked （AdviceFlip：bound 期间建议翻转）

type LeaseState string

const (
	// LeaseIssued DriftGate.Check 放行后签发，尚未绑定到提交路径。
	LeaseIssued LeaseState = "issued"
	// LeaseBound 提交路径已携带此租约，复验进行中。
	LeaseBound LeaseState = "bound"
	// LeaseCommitted 提交复验通过，出金/归集放行。
	LeaseCommitted LeaseState = "committed"
	// LeaseExpired TTL 到期；提交必须拒绝。
	LeaseExpired LeaseState = "expired"
	// LeaseBlocked 建议翻转（AdviceFlip）或 checker 故障；提交必须拒绝。
	LeaseBlocked LeaseState = "blocked"
)

// Terminal 报告状态是否为终态（无法再推进）。
  func (s LeaseState) Terminal() bool {
	  return s == LeaseCommitted || s == LeaseExpired || s == LeaseBlocked
  }

// ─── 建议指纹 ──────────────────────────────────────────────────────────────────
// AdviceFingerprint 封堵建议快照指纹，用于检测翻转（Advice Flip）。
// HasActive=false 代表无活跃建议；AdviceHash 为建议列表的稳定摘要（sha256 hex）。
// 若两次采集 HasActive 或 AdviceHash 不同，视为翻转。

type AdviceFingerprint struct {
	Asset      string `json:"asset"`
	HasActive  bool   `json:"has_active"`
	AdviceHash string `json:"advice_hash"` // sha256(sorted active advice IDs) hex；无建议为 ""
	CapturedAt time.Time `json:"captured_at"`
}

// Equal 判断两指纹是否等价（只比较 HasActive 与 AdviceHash，不比较时间）。
  func (a AdviceFingerprint) Equal(b AdviceFingerprint) bool {
	   return a.HasActive == b.HasActive && a.AdviceHash == b.AdviceHash
  }

// ─── 预检租约 ──────────────────────────────────────────────────────────────────
// InvPrecheckLease DriftGate.Check 放行后签发的预检租约。
// 提交路径必须携带未过期租约并通过 CommitRecheck 后才能放行。
// inv_precheck_id 供外包只读引用，不得被他包修改状态。
type InvPrecheckLease struct {
	LeaseID string `json:"lease_id"` // inv_precheck_id（UUID）
	Asset string     `json:"asset"` // 归属资产（大写，如 BTC）
	Actor string     `json:"actor"` // 发起预检的操作人
	State LeaseState `json:"state"`
	// 建议指纹快照（Check 放行时采集）；CommitRecheck 时核对当前指纹
	FingerprintAtIssue AdviceFingerprint `json:"fingerprint_at_issue"`
	// 签发时间与到期时间
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	// 绑定信息（Bind 时填充）
	BoundAt   *time.Time `json:"bound_at,omitempty"`
	CommitRef string     `json:"commit_ref,omitempty"` // 出金/归集提交的业务 ID，仅供追踪
	// 终态时间与原因
	CommittedAt *time.Time `json:"committed_at,omitempty"`
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	BlockedAt   *time.Time `json:"blocked_at,omitempty"`
	BlockReason string     `json:"block_reason,omitempty"` // "advice_flip"/"checker_unavailable"/"commit_drift"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 深拷贝租约（指针字段）。
   func (l InvPrecheckLease) Copy() InvPrecheckLease {
	  cp := l
	  if l.BoundAt != nil {
		   t := *l.BoundAt
		   cp.BoundAt = &t
	  }
	   if l.CommittedAt != nil {
		   t := *l.CommittedAt
		   cp.CommittedAt = &t
	   }
	  if l.ExpiredAt != nil {
		   t := *l.ExpiredAt
		  cp.ExpiredAt = &t
	   }
	   if l.BlockedAt != nil {
		  t := *l.BlockedAt
		  cp.BlockedAt = &t
	   }
	  return cp
  }

// ─── 漂移证明 ──────────────────────────────────────────────────────────────────
// DriftKind 漂移证明类型。
type DriftKind string

const (
	// DriftAdviceFlip 建议翻转（无→有 或 有→无）。
	DriftAdviceFlip DriftKind = "advice_flip"
	// DriftCheckerDown checker 不可用导致 fail-closed 拒绝。
	DriftCheckerDown DriftKind = "checker_down"
	// DriftCommitNoLease 提交路径无合法租约（未签发或已过期）。
	DriftCommitNoLease DriftKind = "commit_no_lease"
	// DriftCommitFingerMismatch 提交时指纹与签发时指纹不一致。
	DriftCommitFingerMismatch DriftKind = "commit_finger_mismatch"
)

// ProofState 漂移证明状态（只追加，仅用于审计标记）。
type ProofState string

const (
	ProofOpen ProofState = "open" // 刚落地，待审计
	ProofClosed ProofState = "closed" // 已审计确认
)

// InvGateDriftProof 漂移证明，记录一次偏离闸的拒绝事件与现场快照。
// 只追加，不修改；一条租约最多一条 proof（terminated lease 落地后即不变）。
type InvGateDriftProof struct {
	ProofID string `json:"proof_id"` // UUID
	// 关联租约（可能为空，当提交路径无 lease 时）
	LeaseID string `json:"lease_id,omitempty"`
	Asset string `json:"asset"`
	Kind DriftKind `json:"kind"`
	State ProofState `json:"state"`
	// 现场快照
	FingerprintAtIssue  *AdviceFingerprint `json:"fingerprint_at_issue,omitempty"`
	FingerprintAtCommit *AdviceFingerprint `json:"fingerprint_at_commit,omitempty"`
	// 操作人与业务上下文
	Actor string `json:"actor,omitempty"`
	CommitRef string `json:"commit_ref,omitempty"`
	Note      string `json:"note,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 深拷贝证明（指针字段）。

  func (p InvGateDriftProof) Copy() InvGateDriftProof {
	  cp := p
	   if p.FingerprintAtIssue != nil {
		  f := *p.FingerprintAtIssue
		   cp.FingerprintAtIssue = &f
	  }
	   if p.FingerprintAtCommit != nil {
		  f := *p.FingerprintAtCommit
		  cp.FingerprintAtCommit = &f
	  }
	  if p.ClosedAt != nil {
		  t := *p.ClosedAt
		   cp.ClosedAt = &t
	  }
	   return cp
   }
