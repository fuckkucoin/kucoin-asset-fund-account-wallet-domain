package signpipegate

import (
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// 枚举
// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus 描述 InflightLease 在状态机中的位置。

type LeaseStatus string

const (
	LeaseArmed LeaseStatus = "armed" // 已抬世代，尚未进入执行
	LeaseInflight LeaseStatus = "inflight" // 已绑定执行窗口，持有互斥
	LeaseSealed   LeaseStatus = "sealed"   // 成功签出并绑定产物，终态
	LeaseAborted    LeaseStatus = "aborted"    // 故障或运维中止
	LeaseExpired    LeaseStatus = "expired"    // TTL 到期，旧世代不得再签
	LeaseSuperseded LeaseStatus = "superseded" // 被更新世代顶替
)

// FaultStatus 故障案生命周期。
type FaultStatus string

const (
	FaultInjected FaultStatus = "injected" // 已注入，尚未观测到管线反应
	FaultObserved FaultStatus = "observed" // 已观测到失败/拒签
	FaultClosed   FaultStatus = "closed"   // 运维收口
)

// FaultKind 注入种类，仅覆盖签章管线事故面。
type FaultKind string

const (
	FaultSigner FaultKind = "signer"
	FaultExpire FaultKind = "expire"
	FaultFence  FaultKind = "fence"
	FaultActor FaultKind = "actor"
	FaultHSMTimeout        FaultKind = "hsm_timeout"
	FaultCanonicalMismatch FaultKind = "canonical_mismatch"
	FaultDualBroadcast  FaultKind = "dual_broadcast"
	FaultRetryExhausted FaultKind = "retry_exhausted"
	FaultGenerationStale FaultKind = "generation_stale"
	FaultInflightRace FaultKind = "inflight_race"
)

// CloseoutStatus 收口证明生命周期。

type CloseoutStatus string

const (
	CloseoutOpen CloseoutStatus = "open"
	CloseoutProven CloseoutStatus = "proven"
	CloseoutClosed CloseoutStatus = "closed"
)

// CloseoutKind 收口原因。
type CloseoutKind string

const (
	CloseTTL   CloseoutKind = "ttl"
	CloseFault CloseoutKind = "fault"
	CloseAbort CloseoutKind = "abort"
	CloseDoubleGuard CloseoutKind = "double_guard"
	CloseSupersede CloseoutKind = "supersede"
	CloseExhausted CloseoutKind = "exhausted"
)

const DefaultLeaseTTL = 10 * time.Minute
const MaxRetryGenerations = 3

const IsolationMark = "signpipe-retry-gen;not-ledger-apply;not-por;not-balproof;no-double-sign"

// ─────────────────────────────────────────────────────────────────────────────
// RetryGeneration
// ─────────────────────────────────────────────────────────────────────────────
// RetryGeneration 记录 movement×job 重试世代 CAS 计数器。
// CAS 协议：
// Acquire（ArmRetry）:
// UPDATE signpipe_retry_generations
// SET generation = generation + 1, updated_at = NOW(3)
// WHERE movement_id=? AND job_id=? AND generation=?
// -- 影响行数 = 0 → ErrStaleGen；= 1 → 新世代 = old+1
// 并发 Arm 未抢到 CAS 必须失败关闭，不得并行进入 inflight。
type RetryGeneration struct {
	ID string `json:"id"`
	MovementID string `json:"movement_id"`
	JobID string `json:"job_id"`
	Generation int64     `json:"generation"`
	UpdatedAt  time.Time `json:"updated_at"`
}

  func (g RetryGeneration) Copy() RetryGeneration { return g }

   func (g RetryGeneration) Key() string {
	   return NormalizeMovement(g.MovementID) + "|" + strings.TrimSpace(g.JobID)
   }

// ─────────────────────────────────────────────────────────────────────────────
// InflightLease
// ─────────────────────────────────────────────────────────────────────────────
// InflightLease 在途签章租约。
type InflightLease struct {
	ID string `json:"id"`
	InflightLeaseID string `json:"inflight_lease_id"`
	MovementID string `json:"movement_id"`
	JobID      string `json:"job_id"`
	Chain     string `json:"chain"`
	To        string `json:"to"`
	Amount    string `json:"amount"`
	Canonical string `json:"canonical"`
	Nonce     string `json:"nonce"`
	Generation int64 `json:"generation"`
	Status LeaseStatus `json:"status"`
	Actor   string `json:"actor"`
	Retries int    `json:"retries"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken    string    `json:"fence_token"`
	SigHex        string    `json:"sig_hex,omitempty"`
	IsolationMark string    `json:"isolation_mark"`
	Note          string    `json:"note,omitempty"`
	ArmedAt       time.Time `json:"armed_at"`
	InflightAt    time.Time `json:"inflight_at,omitempty"`
	SealedAt      time.Time `json:"sealed_at,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	TerminalAt time.Time `json:"terminal_at,omitempty"`
}

   func (l InflightLease) Copy() InflightLease { return l }

  func (l InflightLease) Terminal() bool {
	  switch l.Status {
	   case LeaseSealed, LeaseAborted, LeaseExpired, LeaseSuperseded:
		  return true
	   default:
		  return false
	  }
   }

  func (l InflightLease) ExpiredAt(now time.Time) bool {
	   if l.ExpiresAt.IsZero() {
		   return false
	  }
	  return !now.Before(l.ExpiresAt)
   }

  func (l InflightLease) SubjectKey() string {
	  return NormalizeMovement(l.MovementID)
  }

// ─────────────────────────────────────────────────────────────────────────────
// FaultCase
// ─────────────────────────────────────────────────────────────────────────────
// FaultCase 故障注入案。
type FaultCase struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	MovementID string `json:"movement_id"`
	JobID string `json:"job_id"`
	Kind FaultKind `json:"kind"`
	Status FaultStatus `json:"status"`
	ObservedState string `json:"observed_state,omitempty"`
	Note          string `json:"note,omitempty"`
	Operator      string `json:"operator"`
	IsolationMark string `json:"isolation_mark"`
	InjectedAt time.Time `json:"injected_at"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
	ClosedAt   time.Time `json:"closed_at,omitempty"`
}

  func (f FaultCase) Copy() FaultCase { return f }

  func (f FaultCase) Blocking() bool {
	  return f.Status == FaultInjected || f.Status == FaultObserved
   }

// ─────────────────────────────────────────────────────────────────────────────
// CloseoutProof
// ─────────────────────────────────────────────────────────────────────────────
// CloseoutProof 在途残留收口证明。

type CloseoutProof struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	MovementID string         `json:"movement_id"`
	JobID      string         `json:"job_id"`
	Kind       CloseoutKind   `json:"kind"`
	Status     CloseoutStatus `json:"status"`
	Digest string `json:"digest"`
	Note   string `json:"note,omitempty"`
	Operator      string `json:"operator,omitempty"`
	IsolationMark string `json:"isolation_mark"`
	OpenedAt time.Time `json:"opened_at"`
	ProvenAt time.Time `json:"proven_at,omitempty"`
	ClosedAt time.Time `json:"closed_at,omitempty"`
}

  func (c CloseoutProof) Copy() CloseoutProof { return c }

   func (c CloseoutProof) Blocking() bool {
	  return c.Status == CloseoutOpen || c.Status == CloseoutProven
   }

// ─────────────────────────────────────────────────────────────────────────────
// SignedArtifact
// ─────────────────────────────────────────────────────────────────────────────
// SignedArtifact 密封签名产物。同 movement 唯一。

type SignedArtifact struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	MovementID string `json:"movement_id"`
	JobID     string `json:"job_id"`
	Canonical string `json:"canonical"`
	SigHex    string `json:"sig_hex"`
	Digest        string `json:"digest"`
	Generation    int64  `json:"generation"`
	IsolationMark string `json:"isolation_mark"`
	SealedAt time.Time `json:"sealed_at"`
}

   func (a SignedArtifact) Copy() SignedArtifact { return a }

// ─────────────────────────────────────────────────────────────────────────────
// 请求
// ─────────────────────────────────────────────────────────────────────────────

type ArmReq struct {
	MovementID string `json:"movement_id"`
	JobID string `json:"job_id"`
	Chain  string `json:"chain"`
	To     string `json:"to"`
	Amount string `json:"amount"`
	Nonce string `json:"nonce"`
	Actor string `json:"actor"`
	RequestedBy string `json:"requested_by,omitempty"`
	ApprovedBy  string `json:"approved_by,omitempty"`
	FenceEpoch  int64  `json:"fence_epoch"`
	FenceToken string        `json:"fence_token"`
	TTL        time.Duration `json:"ttl,omitempty"`
}

type InjectReq struct {
	LeaseID string    `json:"lease_id"`
	Kind    FaultKind `json:"kind"`
	Operator   string `json:"operator"`
	Actor      string `json:"actor"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

type SealReq struct {
	LeaseID string `json:"lease_id"`
	Actor   string `json:"actor"`
	SigHex     string `json:"sig_hex,omitempty"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

type AbortReq struct {
	LeaseID  string `json:"lease_id"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

type CloseoutCloseReq struct {
	ProofID  string `json:"proof_id"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

type FaultCloseReq struct {
	FaultID  string `json:"fault_id"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

// ObservedJob 管线只读观察。

type ObservedJob struct {
	ID string
	MovementID string
	Chain      string
	To         string
	Amount string
	Canonical string
	RequestedBy string
	ApprovedBy  string
	SignedBy    string
	State  string
	SigHex string
	Retries    int
	FenceEpoch int64
	FenceToken string
	ExpiresAt  time.Time
}

   func NormalizeMovement(id string) string {
	  return strings.TrimSpace(id)
   }

   func NormalizeChain(chain string) string {
	   return strings.ToUpper(strings.TrimSpace(chain))
   }

  func NormalizeDest(to string) string {
	  return strings.TrimSpace(to)
  }

  func NormalizeActor(actor string) string {
	  return strings.TrimSpace(actor)
  }

   func NormalizeNonce(nonce string) string {
	   return strings.TrimSpace(nonce)
   }

   func KnownFaultKind(k FaultKind) bool {
	   switch k {
	   case FaultSigner, FaultExpire, FaultFence, FaultActor, FaultHSMTimeout,
		  FaultCanonicalMismatch, FaultDualBroadcast, FaultRetryExhausted,
		   FaultGenerationStale,
		  FaultInflightRace:
		  return true
	  default:
		  return false
	  }
   }

   func KnownCloseoutKind(k CloseoutKind) bool {
	  switch k {
	   case CloseTTL, CloseFault, CloseAbort, CloseDoubleGuard, CloseSupersede, CloseExhausted:
		   return true
	  default:
		  return false
	   }
  }
