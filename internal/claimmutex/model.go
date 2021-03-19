package claimmutex

import "time"

// ─────────────────────────── OutputLease ────────────────────────────────────
// LeaseState 输出效果租约状态机。
// open → claimed : 第一次 Claim 成功持有租约
// open → auto_credited: 第一次自动贷记持有租约
// claimed → blocked : 发现后续同输出动作（含 Recovery），转为封堵态
// auto_credited → blocked : 发现后续同输出动作，转为封堵态
// open → expired : TTL 超期（正常情况下不发生）
type LeaseState string

const (
	LeaseOpen LeaseState = "open"
	LeaseClaimed      LeaseState = "claimed"
	LeaseAutoCredited LeaseState = "auto_credited"
	LeaseBlocked      LeaseState = "blocked"
	LeaseExpired LeaseState = "expired"
)

// DefaultLeaseTTL 认领效果租约默认生存期。
// 超过此时间仍未完成认领则视为过期，扫描 worker 将其标记为 expired。

const DefaultLeaseTTL = 5 * time.Minute

// OutputKey 唯一标识一个链上输出。

type OutputKey struct {
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	OutputIndex int `json:"output_index"`
}

// String 返回 "chain|tx_hash|output_index" 复合键。
  func (k OutputKey) String() string {
	   if k.Chain == "" || k.TxHash == "" {
		  return ""
	  }
	   return k.Chain + "|" + k.TxHash + "|" + itoa(k.OutputIndex)
   }

  func itoa(n int) string {
	  if n == 0 {
		  return "0"
	   }
	  buf := [20]byte{}
	   pos := len(buf)
	  neg := n < 0
	   if neg {
		   n = -n
	  }
	  for n > 0 {
		  pos--
		   buf[pos] = byte('0' + n%10)
		   n /= 10
	   }
	   if neg {
		  pos--
		   buf[pos] = '-'
	   }
	   return string(buf[pos:])
   }

// OutputLease 输出效果世代租约。
// 首次成功 Claim 或自动贷记时由互斥层创建；同输出后续动作须携带
// 同一 LeaseID / Generation 方可接着，否则被拒绝并产出 DupClaimCase。
// 状态转移详见 LeaseState 注释。

type OutputLease struct {
	LeaseID   string    `json:"lease_id"`
	OutputKey OutputKey `json:"output_key"`
	Generation int64      `json:"generation"`
	State      LeaseState `json:"state"`
	WinnerKind  string    `json:"winner_kind"`   // "claim" | "auto_credit"
	WinnerActor string    `json:"winner_actor"`  // user_id 或系统标识
	ClaimCaseID string    `json:"claim_case_id"` // 若双贷发现，关联的 DupClaimCase
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`
	BlockedAt *time.Time `json:"blocked_at,omitempty"`
	BlockReason string `json:"block_reason,omitempty"`
}

// IsExpired 判断租约是否超期。
  func (l OutputLease) IsExpired(now time.Time) bool {
	   return now.After(l.ExpiresAt)
  }

// IsUsable 租约是否可被后续操作携带（open 且未超期）。
  func (l OutputLease) IsUsable(now time.Time) bool {
	  return l.State == LeaseOpen && !l.IsExpired(now)
  }

// IsBlocked 输出是否已进入封堵态（不允许任何新的认领动作）。

  func (l OutputLease) IsBlocked() bool {
	   return l.State == LeaseBlocked || l.State == LeaseExpired
   }

// ─────────────────────────── DupClaimCase ───────────────────────────────────
// DupCaseState 双贷案件状态机。
// open → proven : ops 双人确认双贷属实，Prove(actor1, actor2) 调用
// proven → closed : 修复完成，Close 调用

type DupCaseState string

const (
	DupCaseOpen   DupCaseState = "open"
	DupCaseProven DupCaseState = "proven"
	DupCaseClosed DupCaseState = "closed"
)

// DupClaimCase 同输出双贷案件。
// 当发现同一 OutputKey 存在两条独立贷记（Claim 与 Claim、Claim 与 auto-credit
// 各自入账）时开案，留存事故轨迹。

type DupClaimCase struct {
	CaseID string `json:"case_id"`
	OutputKey   OutputKey `json:"output_key"`
	LeaseID     string    `json:"lease_id"`   // 首个胜出者租约 ID
	FirstKind   string    `json:"first_kind"` // "claim" | "auto_credit"
	FirstActor  string    `json:"first_actor"`
	FirstIdem   string    `json:"first_idem"`  // 首次幂等键
	SecondKind  string    `json:"second_kind"` // 第二腿类型
	SecondActor string    `json:"second_actor"`
	SecondIdem  string `json:"second_idem"`
	DeltaAmount string `json:"delta_amount"` // 双贷总额差
	State DupCaseState `json:"state"`
	DetectedAt time.Time  `json:"detected_at"`
	ProvenAt   *time.Time `json:"proven_at,omitempty"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
	Actor1 string `json:"actor1,omitempty"`
	Actor2    string `json:"actor2,omitempty"`
	CloseNote string `json:"close_note,omitempty"`
}

// ─────────────────────────── AutoCreditBind ─────────────────────────────────
// BindState 自动贷记绑定状态。
// pending → bound : Claim 互斥层确认自动贷记为胜出腿
// pending → rejected : 另一腿（Claim）先胜出，此自动贷记腿被拒绝
// bound → released : 认领效果完成，绑定释放

type BindState string

const (
	BindPending BindState = "pending"
	BindBound    BindState = "bound"
	BindRejected BindState = "rejected"
	BindReleased BindState = "released"
)

// AutoCreditBind 自动贷记绑定记录。
// 自动贷记管道触发时先写 AutoCreditBind（状态 pending）；
// 互斥层在处理 Claim 时检查是否存在同输出的 pending 绑定：
// - 若无：允许 Claim，并将后续到达的自动贷记绑定置为 rejected。
// - 若有：检查绑定世代是否一致，若不一致拒绝 Claim 并开 DupClaimCase。

type AutoCreditBind struct {
	BindID string `json:"bind_id"`
	OutputKey  OutputKey `json:"output_key"`
	Generation int64     `json:"generation"`
	LeaseID    string    `json:"lease_id"`
	ActorID    string    `json:"actor_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount  string    `json:"amount"`
	IdemKey string    `json:"idem_key"`
	State   BindState `json:"state"`
	BoundAt time.Time `json:"bound_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	RejectNote string     `json:"reject_note,omitempty"`
}

// ─────────────────────────── ClaimEffectProof ───────────────────────────────
// ProofState 认领效果证明状态机。
// pending → verified : 互斥层校验效果世代与金额守恒通过
// verified → sealed : ops 或 worker 封印，认领流程视为完成
// pending → failed : 校验失败（世代不符或金额偏差）
// failed → sealed : ops 人工确认偏差并封印

type ProofState string

const (
	ProofPending  ProofState = "pending"
	ProofVerified ProofState = "verified"
	ProofSealed ProofState = "sealed"
	ProofFailed ProofState = "failed"
)

// ClaimEffectProof 认领效果证明。
// 记录首次胜出认领（Claim 或 auto-credit）的路径与金额，
// 后续任何完成标记操作须先确认证明已 verified/sealed，否则返回 ErrProofMissing。
type ClaimEffectProof struct {
	ProofID    string    `json:"proof_id"`
	OutputKey  OutputKey `json:"output_key"`
	LeaseID    string    `json:"lease_id"`
	Generation int64     `json:"generation"`
	ClaimKind string `json:"claim_kind"` // "claim" | "auto_credit"
	ActorID string `json:"actor_id"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	IdemKey string `json:"idem_key"`
	State ProofState `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	SealedAt *time.Time `json:"sealed_at,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
	SealedBy   string `json:"sealed_by,omitempty"`
}

// CanComplete 证明是否允许将认领标记为完成。

  func (p ClaimEffectProof) CanComplete() bool {
	  return p.State == ProofVerified || p.State == ProofSealed
   }

// ─────────────────────────── Request / Response ──────────────────────────────
// AcquireLeaseRequest 首次 Claim 或 auto-credit 抢占输出效果租约。
type AcquireLeaseRequest struct {
	OutputKey OutputKey `json:"output_key"`
	Kind      string    `json:"kind"` // "claim" | "auto_credit"
	ActorID   string    `json:"actor_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount  string        `json:"amount"`
	IdemKey string        `json:"idem_key"`
	TTL     time.Duration `json:"ttl,omitempty"`
}

// GuardRequest 后续同输出动作须携带此请求进行世代核验。

type GuardRequest struct {
	OutputKey  OutputKey `json:"output_key"`
	LeaseID    string    `json:"lease_id"`
	Generation int64     `json:"generation"`
	ActorID string `json:"actor_id"`
	Kind string `json:"kind"`
}

// RegisterAutoCreditRequest 自动贷记管道注册绑定。
type RegisterAutoCreditRequest struct {
	OutputKey OutputKey `json:"output_key"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	IdemKey string `json:"idem_key"`
	ActorID string `json:"actor_id"`
}

// CreateProofRequest 提交认领效果证明。

type CreateProofRequest struct {
	OutputKey OutputKey `json:"output_key"`
	LeaseID   string    `json:"lease_id"`
	Generation int64 `json:"generation"`
	ClaimKind string `json:"claim_kind"`
	ActorID   string `json:"actor_id"`
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	IdemKey string `json:"idem_key"`
}

// CompleteClaimRequest 将认领标记为完成；须携带 ProofID。

type CompleteClaimRequest struct {
	OutputKey OutputKey `json:"output_key"`
	LeaseID string `json:"lease_id"`
	ProofID  string `json:"proof_id"`
	SealedBy string `json:"sealed_by"`
}

// ProveDupCaseRequest ops 双人确认双贷案件。

type ProveDupCaseRequest struct {
	CaseID string `json:"case_id"`
	Actor1 string `json:"actor1"`
	Actor2 string `json:"actor2"`
}

// CloseDupCaseRequest ops 关闭双贷案件（修复完成）。

type CloseDupCaseRequest struct {
	CaseID string `json:"case_id"`
	CloseNote string `json:"close_note"`
	ClosedBy string `json:"closed_by"`
}

// OutputEffectSummary ops 查询接口返回的输出效果摘要。
type OutputEffectSummary struct {
	OutputKey OutputKey         `json:"output_key"`
	Lease     *OutputLease      `json:"lease,omitempty"`
	Proof     *ClaimEffectProof `json:"proof,omitempty"`
	DupCase  *DupClaimCase   `json:"dup_case,omitempty"`
	AutoBind *AutoCreditBind `json:"auto_bind,omitempty"`
}
