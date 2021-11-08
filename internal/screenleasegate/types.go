package screenleasegate

import "time"

// LeaseStatus 描述 ScreenLease 在状态机中的位置。
type LeaseStatus string

const (
	LeaseOpen LeaseStatus = "open" // 已签发，尚未绑定提交
	LeaseBound LeaseStatus = "bound" // 已绑定一次 Submit 意图
	LeaseConsumed LeaseStatus = "consumed" // 提交复验通过并消耗，终态
	LeaseExpired LeaseStatus = "expired" // TTL 到期，不得再提交
	LeaseRevoked LeaseStatus = "revoked" // Halt/Pack 翻转或运维吊销
)

   func (s LeaseStatus) Terminal() bool {
	  return s == LeaseConsumed || s == LeaseExpired || s == LeaseRevoked
  }

   func (s LeaseStatus) CanBind() bool {
	   return s == LeaseOpen
   }

  func (s LeaseStatus) CanConsume() bool {
	   return s == LeaseBound || s == LeaseOpen
   }

// ProofStatus 漂移证明生命周期。
type ProofStatus string

const (
	ProofOpen   ProofStatus = "open"
	ProofProven ProofStatus = "proven"
	ProofClosed ProofStatus = "closed"
)

// ProofKind 漂移原因。仅覆盖 Screen 放行与提交夹缝。

type ProofKind string

const (
	ProofHaltFlip ProofKind = "halt_flip"
	ProofPackFlip ProofKind = "pack_flip"
	ProofModeFlip ProofKind = "mode_flip"
	ProofMissingLease ProofKind = "missing_lease"
	ProofExpired      ProofKind = "expired"
	ProofWindowDrift ProofKind = "window_drift"
	ProofRevoked     ProofKind = "revoked"
	ProofStaleGen    ProofKind = "stale_gen"
	ProofNoAllow     ProofKind = "no_allow"
	ProofAmountDrift ProofKind = "amount_drift"
	ProofDestDrift ProofKind = "dest_drift"
	ProofConcurrent ProofKind = "concurrent_bind"
	ProofFourEye    ProofKind = "four_eye_clear"
)

const DefaultLeaseTTL = 8 * time.Minute
const MaxOpenLeasesPerUser = 8

const IsolationMark = "screen-lease;not-wdtriple;not-wdverify;not-dualctl;not-policypack-formula"

// ScreenGeneration 记录全局 Halt 世代与 Pack/Engine 世代。
// CAS 协议：
// BumpHalt:
// UPDATE risk_screen_generations
// SET halt_gen = halt_gen + 1, halt_on=?, updated_at=NOW(3)
// WHERE id='singleton' AND halt_gen=?
// BumpPack:
// UPDATE risk_screen_generations
// SET pack_gen = pack_gen + 1, pack=?, mode=?, pack_fp=?, updated_at=NOW(3)
// WHERE id='singleton' AND pack_gen=?
// 影响行数 0 → 并发翻转失败关闭，旧租约仍按旧世代复验并拒。
type ScreenGeneration struct {
	ID      string `json:"id"`
	HaltGen int64  `json:"halt_gen"`
	HaltOn  bool   `json:"halt_on"`
	PackGen int64  `json:"pack_gen"`
	Pack string `json:"pack"`
	Mode string `json:"mode"`
	PackFP string `json:"pack_fp"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (g ScreenGeneration) Copy() ScreenGeneration { return g }

// ScreenLease Screen Allow 后签发的放行租约。
// 签发时冻结：
// - Pack / Mode / PackFP（策略包指纹）
// - HaltGen（签发瞬间的 Halt 世代）
// - WindowDigest（速度窗与目的地窗摘要）
// - Amount / Dest / Asset / UserID
// Submit 必须逐项复验；任一漂移 fail-closed 并落证明。

type ScreenLease struct {
	ID string `json:"screen_lease_id"`
	UserID string `json:"user_id"`
	Dest  string `json:"dest"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Kind   string `json:"kind"`
	Pack string `json:"pack"`
	Mode string `json:"mode"`
	PackFP  string `json:"pack_fp"`
	HaltGen int64  `json:"halt_gen"`
	PackGen int64  `json:"pack_gen"`
	WindowDigest string `json:"window_digest"`
	Velocity1h   int    `json:"velocity_1h"`
	Velocity24h int `json:"velocity_24h"`
	DestCount   int `json:"dest_count"`
	DestCount24h int         `json:"dest_count_24h"`
	Nonce        string      `json:"nonce"`
	Status       LeaseStatus `json:"status"`
	IsolationMark string `json:"isolation_mark"`
	Note     string     `json:"note,omitempty"`
	IssuedAt time.Time  `json:"issued_at"`
	BoundAt  *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	TerminalAt *time.Time `json:"terminal_at,omitempty"`
	BindTicket string     `json:"bind_ticket,omitempty"`
}

  func (l ScreenLease) Copy() ScreenLease {
	  out := l
	   if l.BoundAt != nil {
		  t := *l.BoundAt
		   out.BoundAt = &t
	  }
	  if l.ConsumedAt != nil {
		   t := *l.ConsumedAt
		   out.ConsumedAt = &t
	   }
	   if l.TerminalAt != nil {
		   t := *l.TerminalAt
		  out.TerminalAt = &t
	  }
	   return out
  }

  func (l ScreenLease) Alive(now time.Time) bool {
	   if l.Status.Terminal() {
		   return false
	  }
	  if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
		  return false
	  }
	   return true
  }

// ScreenDriftProof 策略漂移证明。同 lease 可多案，同 (lease,kind,gen) 幂等。

type ScreenDriftProof struct {
	ID      string `json:"proof_id"`
	LeaseID string `json:"screen_lease_id"`
	UserID  string `json:"user_id"`
	Kind         ProofKind   `json:"kind"`
	Status       ProofStatus `json:"status"`
	ExpectedHalt int64       `json:"expected_halt_gen"`
	ObservedHalt int64  `json:"observed_halt_gen"`
	ExpectedPack string `json:"expected_pack"`
	ObservedPack string `json:"observed_pack"`
	ExpectedFP   string `json:"expected_pack_fp"`
	ObservedFP   string `json:"observed_pack_fp"`
	Digest        string `json:"digest"`
	IsolationMark string `json:"isolation_mark"`
	Note      string `json:"note,omitempty"`
	OperatorA string `json:"operator_a,omitempty"`
	OperatorB string `json:"operator_b,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

   func (p ScreenDriftProof) Copy() ScreenDriftProof {
	  out := p
	   if p.ProvenAt != nil {
		   t := *p.ProvenAt
		   out.ProvenAt = &t
	   }
	   if p.ClosedAt != nil {
		   t := *p.ClosedAt
		  out.ClosedAt = &t
	   }
	  return out
  }

   func (p ScreenDriftProof) BlocksSubmit() bool {
	   return p.Status == ProofOpen || p.Status == ProofProven
  }

// IssueReq Screen 签发请求。字段一致 riskgate.ScreenReq，额外携带 nonce。
type IssueReq struct {
	UserID string `json:"user_id"`
	Pack string `json:"pack"`
	Kind string `json:"kind"`
	Amount string `json:"amount"`
	Asset       string `json:"asset"`
	Dest        string `json:"dest"`
	Allowlisted bool   `json:"allowlisted"`
	HourUTC int `json:"hour_utc"`
	Weekday string `json:"weekday"`
	Nonce   string `json:"nonce"`
	TTLSeconds int `json:"ttl_seconds"`
}

// SubmitReq 出金提交复验。必须携带 screen_lease_id。

type SubmitReq struct {
	LeaseID string `json:"screen_lease_id"`
	UserID  string `json:"user_id"`
	Dest    string `json:"dest"`
	Asset string `json:"asset"`
	Amount  string `json:"amount"`
	Ticket  string `json:"ticket"`
	Actor   string `json:"actor"`
	Consume bool   `json:"consume"`
}

// FlipPackReq 切换策略包/引擎模式，抬 Pack 世代。
type FlipPackReq struct {
	Pack     string `json:"pack"`
	Mode     string `json:"mode"`
	Operator string `json:"operator"`
}

// SetHaltReq 抬 Halt 世代。

type SetHaltReq struct {
	On       bool   `json:"on"`
	Operator string `json:"operator"`
}

// FourEyeReq 四眼清障关闭漂移证明。

type FourEyeReq struct {
	ProofID   string `json:"proof_id"`
	OperatorA string `json:"operator_a"`
	OperatorB string `json:"operator_b"`
	Reason    string `json:"reason"`
}

// WindowSnap 速度窗与目的地窗只读快照，来自 riskgate Windows。
type WindowSnap struct {
	Velocity1h  int
	Velocity24h int
	DestCount    int
	DestCount24h int
}

// ScreenSnap riskgate Screen 只读结果，不含三联闸/OTP/双控字段。
type ScreenSnap struct {
	Allow  bool
	Reason string
	Pack string
	Mode string
}

// GateStats 运维计数。

type GateStats struct {
	Issued int64 `json:"issued"`
	Denied   int64 `json:"denied"`
	Bound    int64 `json:"bound"`
	Consumed int64 `json:"consumed"`
	Expired  int64 `json:"expired"`
	Revoked  int64 `json:"revoked"`
	Proofs   int64 `json:"proofs"`
	SubmitOK int64 `json:"submit_ok"`
	SubmitDeny int64 `json:"submit_deny"`
	HaltBumps  int64 `json:"halt_bumps"`
	PackBumps  int64 `json:"pack_bumps"`
}
