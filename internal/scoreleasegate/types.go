package scoreleasegate

import "time"

// LeaseStatus 描述 ScoreLease 在状态机中的位置。
type LeaseStatus string

const (
	LeaseIssued LeaseStatus = "issued" // 已签发，尚未绑定提交
	LeaseBound    LeaseStatus = "bound"    // 已绑定一次 Submit 意图
	LeaseConsumed LeaseStatus = "consumed" // 提交复验通过并消耗，终态
	LeaseExpired LeaseStatus = "expired" // TTL 到期，不得再提交
	LeaseRevoked LeaseStatus = "revoked" // Halt/Pack/Mode/Window 翻转或运维吊销
)

  func (s LeaseStatus) Terminal() bool {
	   return s == LeaseConsumed || s == LeaseExpired || s == LeaseRevoked
   }

  func (s LeaseStatus) CanBind() bool {
	   return s == LeaseIssued
   }

   func (s LeaseStatus) CanConsume() bool {
	  return s == LeaseBound || s == LeaseIssued
  }

   func (s LeaseStatus) OpenLike() bool {
	  return s == LeaseIssued || s == LeaseBound
   }

// ProofStatus 漂移证明生命周期。
type ProofStatus string

const (
	ProofOpen   ProofStatus = "open"
	ProofProven ProofStatus = "proven"
	ProofClosed ProofStatus = "closed"
)

// ProofKind 漂移原因。仅覆盖 riskscore Eval 放行与出金提交夹缝。

type ProofKind string

const (
	ProofHaltFlip ProofKind = "halt_flip"
	ProofPackFlip ProofKind = "pack_flip"
	ProofModeFlip     ProofKind = "mode_flip"
	ProofWindowDrift  ProofKind = "window_drift"
	ProofMissingLease ProofKind = "missing_lease"
	ProofExpired      ProofKind = "expired"
	ProofRevoked      ProofKind = "revoked"
	ProofStaleGen ProofKind = "stale_gen"
	ProofNoAllow     ProofKind = "no_allow"
	ProofShadowMode  ProofKind = "shadow_mode"
	ProofAmountDrift ProofKind = "amount_drift"
	ProofDestDrift   ProofKind = "dest_drift"
	ProofChainDrift  ProofKind = "chain_drift"
	ProofAssetDrift ProofKind = "asset_drift"
	ProofConcurrent ProofKind = "concurrent_bind"
	ProofFourEye ProofKind = "four_eye_clear"
	ProofFingerprint ProofKind = "fingerprint_drift"
)

const DefaultLeaseTTL = 6 * time.Minute

const MaxOpenLeasesPerUser = 6
const IsolationMark = "score-lease;not-riskgate-screen;not-destgraph-limit;not-policypack-weight;not-limitgate"

// ScoreGeneration 记录全局 Halt/Pack/Mode/Window 世代。
// CAS 协议：
// BumpHalt:
// UPDATE risk_score_generations
// SET halt_gen = halt_gen + 1, halt_on=?, updated_at=NOW(3)
// WHERE id='singleton' AND halt_gen=?
// BumpPack:
// UPDATE risk_score_generations
// SET pack_gen = pack_gen + 1, pack=?, pack_fp=?, updated_at=NOW(3)
// WHERE id='singleton' AND pack_gen=?
// BumpMode:
// UPDATE risk_score_generations
// SET mode_gen = mode_gen + 1, mode=?, updated_at=NOW(3)
// WHERE id='singleton' AND mode_gen=?
// BumpWindow:
// UPDATE risk_score_generations
// SET window_gen = window_gen + 1, window_digest=?, updated_at=NOW(3)
// WHERE id='singleton' AND window_gen=?
// 影响行数 0 → 并发翻转失败关闭，旧租约仍按旧世代复验并拒。

type ScoreGeneration struct {
	ID string `json:"id"`
	HaltGen int64 `json:"halt_gen"`
	HaltOn  bool  `json:"halt_on"`
	PackGen int64 `json:"pack_gen"`
	Pack    string `json:"pack"`
	PackFP  string `json:"pack_fp"`
	ModeGen int64  `json:"mode_gen"`
	Mode      string `json:"mode"`
	WindowGen int64  `json:"window_gen"`
	WindowDigest string    `json:"window_digest"`
	UpdatedAt    time.Time `json:"updated_at"`
}

   func (g ScoreGeneration) Copy() ScoreGeneration { return g }

// ScoreLease Eval Allow 后签发的提交绑定租约。
// 签发时冻结：
// - Pack / Mode / PackFP（策略包与引擎指纹）
// - HaltGen / PackGen / ModeGen / WindowGen
// - WindowDigest（速度窗与目的地窗摘要）
// - Asset / Chain / Dest / Amount / UserID 指纹
// Submit 必须逐项复验；任一漂移 fail-closed 并落 ScoreDriftProof。
// Shadow 命中不得签发 Enforce 租约。

type ScoreLease struct {
	ID     string `json:"score_lease_id"`
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Dest   string `json:"dest"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Kind   string `json:"kind"`
	Pack string `json:"pack"`
	Mode   string `json:"mode"`
	PackFP string `json:"pack_fp"`
	HaltGen int64 `json:"halt_gen"`
	PackGen int64 `json:"pack_gen"`
	ModeGen int64 `json:"mode_gen"`
	WindowGen    int64  `json:"window_gen"`
	WindowDigest string `json:"window_digest"`
	Velocity1h   int    `json:"velocity_1h"`
	Velocity24h  int    `json:"velocity_24h"`
	DestCount    int `json:"dest_count"`
	DestCount24h int `json:"dest_count_24h"`
	ClusterSize int `json:"cluster_size"`
	Correlated  int `json:"correlated"`
	NewDests    int `json:"new_dests_today"`
	Fingerprint string `json:"asset_chain_dest_fp"`
	Nonce string `json:"nonce"`
	Status LeaseStatus `json:"status"`
	IsolationMark string `json:"isolation_mark"`
	Note          string `json:"note,omitempty"`
	IssuedAt time.Time `json:"issued_at"`
	BoundAt *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	TerminalAt *time.Time `json:"terminal_at,omitempty"`
	BindTicket string `json:"bind_ticket,omitempty"`
	EvalAllow bool `json:"eval_allow"`
	EvalReason string `json:"eval_reason,omitempty"`
}

  func (l ScoreLease) Copy() ScoreLease {
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

   func (l ScoreLease) ExpiredAt(now time.Time) bool {
	   if l.Status == LeaseExpired {
		  return true
	   }
	  if l.ExpiresAt.IsZero() {
		  return false
	  }
	  return !now.Before(l.ExpiresAt)
   }

// ScoreDriftProof Allow 与 Submit 夹缝证明。
type ScoreDriftProof struct {
	ID string `json:"score_drift_proof_id"`
	LeaseID string    `json:"score_lease_id,omitempty"`
	UserID  string    `json:"user_id"`
	Kind    ProofKind `json:"kind"`
	Status ProofStatus `json:"status"`
	ExpectedHalt int64  `json:"expected_halt_gen"`
	ObservedHalt int64  `json:"observed_halt_gen"`
	ExpectedPack string `json:"expected_pack"`
	ObservedPack string `json:"observed_pack"`
	ExpectedMode string `json:"expected_mode"`
	ObservedMode string `json:"observed_mode"`
	ExpectedFP   string `json:"expected_fp"`
	ObservedFP string `json:"observed_fp"`
	ExpectedWin string `json:"expected_window_digest"`
	ObservedWin string `json:"observed_window_digest"`
	ExpectedWinGen int64 `json:"expected_window_gen"`
	ObservedWinGen int64 `json:"observed_window_gen"`
	IsolationMark string `json:"isolation_mark"`
	Note string `json:"note,omitempty"`
	Digest   string     `json:"digest"`
	OpenedAt time.Time  `json:"opened_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	CloserA     string     `json:"closer_a,omitempty"`
	CloserB     string     `json:"closer_b,omitempty"`
	CloseReason string     `json:"close_reason,omitempty"`
}

  func (p ScoreDriftProof) Copy() ScoreDriftProof {
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

  func (p ScoreDriftProof) BlocksSubmit() bool {
	   return p.Status == ProofOpen || p.Status == ProofProven
  }

// WindowSnap 冻结 Eval 当时的速度窗。
type WindowSnap struct {
	Velocity1h int
	Velocity24h int
	DestCount    int
	DestCount24h int
	ClusterSize  int
	Correlated int
	NewDests int
	Digest   string
}

// EvalSnap riskscore.Eval 只读快照。Shadow Allow 不得冒充 Enforce。

type EvalSnap struct {
	Allow bool
	Reason string
	Pack   string
	Mode   string
	Hold   bool
	UserID string
	Chain  string
	Dest   string
	Asset  string
	Amount string
	Window WindowSnap
}

// IssueReq EvalWithLease 入参。
type IssueReq struct {
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Dest   string `json:"dest"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Pack  string `json:"pack"`
	Nonce string `json:"nonce"`
	Kind  string `json:"kind"`
	Allowlisted bool `json:"allowlisted"`
	HourUTC     int  `json:"hour_utc"`
	FirstDest bool `json:"first_dest"`
	TTLSeconds int `json:"ttl_seconds"`
	Note string `json:"note"`
}

// SubmitReq 出金冻结/广播提交复验。
type SubmitReq struct {
	ScoreLeaseID string `json:"score_lease_id"`
	UserID       string `json:"user_id"`
	Chain        string `json:"chain"`
	Dest   string `json:"dest"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Ticket string `json:"ticket"`
	Actor string `json:"actor"`
	Consume bool `json:"consume"`
}

// SetHaltReq 抬升 Halt 世代。

type SetHaltReq struct {
	On     bool   `json:"on"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// SetPackReq 更换策略包。
type SetPackReq struct {
	Pack   string `json:"pack"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// SetModeReq 切换 Enforce/Shadow。
type SetModeReq struct {
	Mode string `json:"mode"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

// BumpWindowReq 窗口计数跃迁。
type BumpWindowReq struct {
	Digest string `json:"digest"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// FourEyeReq 关闭漂移证明。

type FourEyeReq struct {
	ProofID string `json:"proof_id"`
	ActorA string `json:"actor_a"`
	ActorB string `json:"actor_b"`
	Reason string `json:"reason"`
}

// RevokeReq 运维吊销。
type RevokeReq struct {
	ScoreLeaseID string `json:"score_lease_id"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

// AuditEvent 租约审计条目。

type AuditEvent struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	LeaseID string    `json:"score_lease_id,omitempty"`
	ProofID string    `json:"proof_id,omitempty"`
	UserID  string    `json:"user_id,omitempty"`
	Actor   string    `json:"actor,omitempty"`
	Note    string    `json:"note,omitempty"`
	At      time.Time `json:"at"`
}

// GateStats 运维计数。

type GateStats struct {
	Issued   int64 `json:"issued"`
	Bound    int64 `json:"bound"`
	Consumed int64 `json:"consumed"`
	Denied int64 `json:"denied"`
	Expired int64 `json:"expired"`
	Revoked int64 `json:"revoked"`
	Proofs    int64 `json:"proofs"`
	HaltBumps int64 `json:"halt_bumps"`
	PackBumps int64 `json:"pack_bumps"`
	ModeBumps int64 `json:"mode_bumps"`
	WindowBump int64 `json:"window_bumps"`
}

// ExportBundle 证明导出包。

type ExportBundle struct {
	Lease  ScoreLease        `json:"lease"`
	Proofs []ScoreDriftProof `json:"proofs"`
	Gen ScoreGeneration `json:"generation"`
	Mark string `json:"isolation_mark"`
	Host string `json:"host"`
}

// ReplayStep golden 回放步骤。
type ReplayStep struct {
	Op string `json:"op"`
	Req    map[string]string `json:"req,omitempty"`
	Expect string            `json:"expect,omitempty"`
	Kind string `json:"kind,omitempty"`
	Advance string `json:"advance,omitempty"`
}
