package holdopsgate

import "time"

// LeaseStatus 描述 HoldOpsLease 在状态机中的位置。

type LeaseStatus string

const (
	LeaseOpen     LeaseStatus = "open"
	LeaseBound    LeaseStatus = "bound"
	LeaseConsumed LeaseStatus = "consumed"
	LeaseExpired LeaseStatus = "expired"
	LeaseRevoked LeaseStatus = "revoked"
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

  func (s LeaseStatus) Writable() bool {
	   return s == LeaseOpen || s == LeaseBound
   }

// ProofStatus 竞态证明生命周期。
type ProofStatus string

const (
	ProofOpen   ProofStatus = "open"
	ProofProven ProofStatus = "proven"
	ProofClosed ProofStatus = "closed"
)

// ProofKind 竞态与失败原因。仅覆盖 ops HTTP 世代事故面。
type ProofKind string

const (
	ProofDualRelease ProofKind = "dual_release"
	ProofReleaseClose  ProofKind = "release_vs_gap_close"
	ProofGapOpenToken  ProofKind = "gap_close_open_token"
	ProofAckNotRelease ProofKind = "ack_not_release"
	ProofExpiredLease  ProofKind = "expired_lease"
	ProofRevokedLease  ProofKind = "revoked_lease"
	ProofStaleGen      ProofKind = "stale_ops_gen"
	ProofMissingLease  ProofKind = "missing_lease"
	ProofConcurrent    ProofKind = "concurrent_ops"
	ProofTokenMismatch ProofKind = "token_mismatch"
	ProofGapMismatch ProofKind = "gap_mismatch"
	ProofFourEye   ProofKind = "four_eye_clear"
	ProofIsolation ProofKind = "isolation_violation"
	ProofQuota ProofKind = "lease_quota"
	ProofWorkerExpire ProofKind = "worker_expire"
	ProofConsistency ProofKind = "gap_token_inconsistent"
)

// OpsKind 写操作种类。Ack 不得映射为 Release 或 Close。
type OpsKind string

const (
	OpsPlace   OpsKind = "place"
	OpsRelease OpsKind = "release"
	OpsGapAck  OpsKind = "gap_ack"
	OpsGapClose OpsKind = "gap_close"
	OpsRevoke   OpsKind = "revoke"
	OpsExpire   OpsKind = "expire"
	OpsFourEye OpsKind = "four_eye"
)

const DefaultLeaseTTL = 6 * time.Minute

const MaxOpenLeasesPerToken = 4
const MaxOpenLeasesPerActor = 16
const IsolationMark = "hold-ops-gen;not-holdcat-matrix;not-holdpatrol-advice;not-amlhold-fsm"

// HoldOpsGeneration 记录全局 hold_ops_gen。
// CAS 协议：
// BumpOps:
// UPDATE hold_ops_generations
// SET ops_gen = ops_gen + 1, updated_at=NOW(3)
// WHERE id='singleton' AND ops_gen=?
// 影响行数 0 → 并发抬世代失败关闭，旧租约按旧世代复验并拒。
type HoldOpsGeneration struct {
	ID string `json:"id"`
	OpsGen  int64  `json:"hold_ops_gen"`
	FenceOn bool   `json:"fence_on"`
	Note    string `json:"note,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (g HoldOpsGeneration) Copy() HoldOpsGeneration { return g }

// HoldOpsLease 单次运维写路径租约。
// 签发时冻结：
// - OpsGen（签发瞬间的 hold_ops_gen）
// - TokenID / GapID / UserID / Asset / Amount
// - Kind（place/release/gap_ack/gap_close）
// - Actor / Nonce
// 写路径必须逐项复验；世代漂移或终态租约失败即关。
type HoldOpsLease struct {
	ID string `json:"ops_lease_id"`
	OpsID  string  `json:"ops_id"`
	Kind   OpsKind `json:"kind"`
	UserID string  `json:"user_id"`
	Asset  string  `json:"asset"`
	Amount string  `json:"amount"`
	TokenID string `json:"token_id"`
	GapID   string `json:"gap_id,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Actor  string      `json:"actor"`
	Nonce  string      `json:"nonce"`
	OpsGen int64       `json:"hold_ops_gen"`
	Status LeaseStatus `json:"status"`
	IsolationMark string    `json:"isolation_mark"`
	Note          string    `json:"note,omitempty"`
	IssuedAt      time.Time `json:"issued_at"`
	BoundAt    time.Time `json:"bound_at,omitempty"`
	ConsumedAt time.Time `json:"consumed_at,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
	TerminalAt time.Time `json:"terminal_at,omitempty"`
	BindTicket string `json:"bind_ticket,omitempty"`
}

   func (l HoldOpsLease) Copy() HoldOpsLease { return l }

  func (l HoldOpsLease) ExpiredAt(now time.Time) bool {
	   if l.Status == LeaseExpired {
		  return true
	   }
	   if l.ExpiresAt.IsZero() {
		   return false
	   }
	  return !now.Before(l.ExpiresAt) && !l.Status.Terminal()
  }

// HoldOpsRaceProof 双释 / 误关 / 夹缝拒操作证明。
type HoldOpsRaceProof struct {
	ID      string `json:"proof_id"`
	LeaseID string `json:"ops_lease_id,omitempty"`
	OpsID   string `json:"ops_id,omitempty"`
	TokenID string `json:"token_id,omitempty"`
	GapID  string    `json:"gap_id,omitempty"`
	UserID string    `json:"user_id"`
	Kind   ProofKind `json:"kind"`
	Status ProofStatus `json:"status"`
	ExpectedGen int64  `json:"expected_gen"`
	ObservedGen int64  `json:"observed_gen"`
	WinnerLease string `json:"winner_lease,omitempty"`
	LoserLease  string `json:"loser_lease,omitempty"`
	OpenTokenCount int       `json:"open_token_count,omitempty"`
	Note           string    `json:"note,omitempty"`
	IsolationMark  string    `json:"isolation_mark"`
	OpenedAt       time.Time `json:"opened_at"`
	ProvenAt    time.Time `json:"proven_at,omitempty"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
	EyeA        string    `json:"eye_a,omitempty"`
	EyeB        string    `json:"eye_b,omitempty"`
	ClearReason string    `json:"clear_reason,omitempty"`
}

  func (p HoldOpsRaceProof) Copy() HoldOpsRaceProof { return p }

   func (p HoldOpsRaceProof) BlocksWrites() bool {
	  return p.Status == ProofOpen || p.Status == ProofProven
  }

// HoldOpsAudit 不可重放审计行。
type HoldOpsAudit struct {
	ID string `json:"audit_id"`
	OpsID   string `json:"ops_id"`
	LeaseID string `json:"ops_lease_id"`
	TokenID string `json:"token_id"`
	GapID string `json:"gap_id,omitempty"`
	Gen   int64  `json:"hold_ops_gen"`
	Actor string  `json:"actor"`
	Kind  OpsKind `json:"kind"`
	Outcome string `json:"outcome"`
	ProofID string `json:"proof_id,omitempty"`
	Note    string `json:"note,omitempty"`
	At time.Time `json:"at"`
	ReplayKey string `json:"replay_key"`
}

   func (a HoldOpsAudit) Copy() HoldOpsAudit { return a }

// ShadowToken 只读邻接 token 投影。不等于 holdcat.Row 会计内核。
type ShadowToken struct {
	ID     string `json:"token_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	Ref    string `json:"ref,omitempty"`
	GapID  string `json:"gap_id,omitempty"`
	Open   bool   `json:"open"`
}

// ShadowGap 只读缺口投影。Ack 仅改 Acked，不改 Closed，不改 token.Open。
type ShadowGap struct {
	ID string `json:"gap_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount    string `json:"amount"`
	Acked     bool   `json:"acked"`
	Closed    bool   `json:"closed"`
	OpenCount int    `json:"open_token_count"`
}

// IssueReq 签发租约。
type IssueReq struct {
	Kind OpsKind `json:"kind"`
	UserID string `json:"user_id"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	TokenID string `json:"token_id"`
	GapID   string `json:"gap_id"`
	Ref     string `json:"ref"`
	Reason string `json:"reason"`
	Actor string `json:"actor"`
	Nonce string `json:"nonce"`
	TTLSec int    `json:"ttl_sec,omitempty"`
	Note   string `json:"note,omitempty"`
}

// ExecReq 在租约上执行 Place/Release/Ack/Close。

type ExecReq struct {
	LeaseID string  `json:"ops_lease_id"`
	Kind    OpsKind `json:"kind"`
	UserID  string  `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	TokenID string `json:"token_id"`
	GapID  string `json:"gap_id"`
	Actor  string `json:"actor"`
	Ticket string `json:"ticket"`
	Consume bool `json:"consume"`
}

// FourEyeReq 四眼清障。
type FourEyeReq struct {
	ProofID string `json:"proof_id"`
	Eye    string `json:"eye"`
	Reason string `json:"reason"`
}

// BumpReq 运维抬世代。
type BumpReq struct {
	Expect int64 `json:"expect_gen"`
	Fence bool `json:"fence_on"`
	Actor string `json:"actor"`
	Note  string `json:"note"`
}

// ExpireReq 手工过期扫描。
type ExpireReq struct {
	Actor string `json:"actor"`
	Limit int `json:"limit"`
}

// Stats 计数快照。

type Stats struct {
	Issued   int64 `json:"issued"`
	Bound    int64 `json:"bound"`
	Consumed int64 `json:"consumed"`
	Denied   int64 `json:"denied"`
	Proofs int64 `json:"proofs"`
	Expired     int64 `json:"expired"`
	Revoked     int64 `json:"revoked"`
	DualRelease int64 `json:"dual_release"`
	GapBlocked int64 `json:"gap_blocked"`
	AckSafe    int64 `json:"ack_safe"`
	GenBumps   int64 `json:"gen_bumps"`
	WorkerTicks int64 `json:"worker_ticks"`
	OpenLeases int   `json:"open_leases"`
	OpenProofs int   `json:"open_proofs"`
	HoldOpsGen int64 `json:"hold_ops_gen"`
}
