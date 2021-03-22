package cerdecidegate

import "time"

// LeaseState Decide 租约状态。

type LeaseState string

const (
	LeaseIssued LeaseState = "issued"
	LeaseBound  LeaseState = "bound"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired  LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

// Terminal 终态不可逆。
   func (s LeaseState) Terminal() bool {
	  return s == LeaseConsumed || s == LeaseExpired || s == LeaseRevoked
  }

// CanBind issued 才可绑 Exec。
   func (s LeaseState) CanBind() bool { return s == LeaseIssued }

// CanConsume bound 才可记为已提交。
   func (s LeaseState) CanConsume() bool { return s == LeaseBound }

// Open 未终态且可被翻转吊销。

  func (s LeaseState) Open() bool {
	  return s == LeaseIssued || s == LeaseBound
   }

// ProofKind 漂移/拒执行证明类型。
type ProofKind string

const (
	ProofPolicyDrift ProofKind = "policy_drift"
	ProofRiskRaise ProofKind = "risk_raise"
	ProofFieldDrift ProofKind = "field_drift"
	ProofSnapDrift ProofKind = "snap_drift"
	ProofExpired   ProofKind = "expired_lease"
	ProofRevoked   ProofKind = "revoked_lease"
	ProofNoLease      ProofKind = "no_lease"
	ProofExemptMisuse ProofKind = "exempt_misuse"
	ProofTTLExpired ProofKind = "ttl_expired"
	ProofGenStale   ProofKind = "generation_stale"
	ProofForceFlip   ProofKind = "force_cold_flip"
	ProofThreshDrift ProofKind = "threshold_drift"
	ProofSharesDrift ProofKind = "shares_drift"
	ProofBindOK      ProofKind = "bind_ok"
	ProofConsumeOK   ProofKind = "consume_ok"
	ProofOpsRevoke ProofKind = "ops_revoke"
	ProofDupBind   ProofKind = "dup_bind"
)

// FlipKind 策略包或目的地风险翻转。

type FlipKind string

const (
	FlipPolicyPack FlipKind = "put_policy_pack"
	FlipForceCold  FlipKind = "put_force_cold"
	FlipThreshold FlipKind = "put_threshold"
	FlipShares    FlipKind = "put_shares"
	FlipDestRisk FlipKind = "raise_dest_risk"
	FlipGeneration FlipKind = "put_generation"
)

// RiskRank 目的地风险档序。抬升只认更大档。
   func RiskRank(s string) int {
	   switch s {
	   case "low":
		   return 1
	   case "medium", "med":
		  return 2
	   case "high":
		  return 3
	   case "critical", "sanctioned":
		   return 4
	   default:
		   return 0
	  }
   }

// PolicyFingerprint 决策时钉扎的策略+门槛+风险指纹。
// 不改写胜出排序与出厂底档加成，只记录当时结果。
type PolicyFingerprint struct {
	PolicyID string `json:"policy_id"`
	ForceCold bool `json:"force_cold"`
	Threshold int `json:"threshold"`
	Shares int `json:"shares"`
	BindMode string `json:"bind_mode"`
	DestRisk string `json:"dest_risk"`
	Required bool `json:"required"`
	PackDigest string `json:"pack_digest"`
	LineDigest string `json:"line_digest"`
	SnapDigest string `json:"snap_digest"`
	Digest     string `json:"digest"`
	Generation int64 `json:"generation"`
}

// Copy 值拷贝。

   func (f PolicyFingerprint) Copy() PolicyFingerprint { return f }

// LineFact 供租约绑定的出金事实，只读钉扎。
type LineFact struct {
	BatchID string `json:"batch_id"`
	Asset   string `json:"asset"`
	Chain string `json:"chain"`
	Amount   string `json:"amount"`
	Dest     string `json:"dest"`
	DestRisk string `json:"dest_risk"`
	Side string `json:"side"`
	Purpose string `json:"purpose"`
	Approved bool `json:"approved"`
}

// Copy 值拷贝。
  func (l LineFact) Copy() LineFact { return l }

// DecidePin 瞬时决策钉扎（Required/Threshold/Shares/PolicyID）。

type DecidePin struct {
	Required bool `json:"required"`
	PolicyID string `json:"policy_id"`
	Threshold int    `json:"threshold"`
	Shares    int    `json:"shares"`
	BindMode  string `json:"bind_mode"`
	DestRisk string `json:"dest_risk"`
	Reason   string `json:"reason"`
}

// Copy 值拷贝。
  func (d DecidePin) Copy() DecidePin { return d }

// DecideLease Decide/DecideSnap 在 Required=true 时签发的租约。
type DecideLease struct {
	LeaseID string `json:"decide_lease_id"`
	BatchID string `json:"batch_id"`
	Asset string `json:"asset"`
	Chain  string `json:"chain"`
	Amount string `json:"amount"`
	Dest   string `json:"dest"`
	DestRisk string `json:"dest_risk"`
	Side string `json:"side"`
	Purpose  string `json:"purpose"`
	PolicyID string `json:"policy_id"`
	Threshold int `json:"threshold"`
	Shares int `json:"shares"`
	BindMode string `json:"bind_mode"`
	Required bool       `json:"required"`
	State    LeaseState `json:"state"`
	TTLSec   int64      `json:"ttl_sec"`
	Idem     string     `json:"idempotency_key"`
	Actor      string `json:"actor"`
	Generation int64  `json:"generation"`
	FlipEpoch  int64  `json:"flip_epoch"`
	ExecRef    string `json:"exec_ref,omitempty"`
	IssuedAt time.Time `json:"issued_at"`
	BoundAt time.Time `json:"bound_at,omitempty"`
	ConsumedAt time.Time `json:"consumed_at,omitempty"`
	ExpiredAt   time.Time         `json:"expired_at,omitempty"`
	RevokedAt   time.Time         `json:"revoked_at,omitempty"`
	RevokeWhy   string            `json:"revoke_why,omitempty"`
	Fingerprint PolicyFingerprint `json:"fingerprint"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 租约值拷贝。
   func (l DecideLease) Copy() DecideLease {
	   out := l
	   out.Fingerprint = l.Fingerprint.Copy()
	   return out
  }

// SameIdemPayload 幂等键复用时事实必须一致。

  func SameIdemPayload(cur, want DecideLease) bool {
	  return cur.BatchID == want.BatchID &&
		   cur.Asset == want.Asset &&
		   cur.Chain == want.Chain &&
		  cur.Amount == want.Amount &&
		  cur.Dest == want.Dest &&
		  cur.PolicyID == want.PolicyID &&
		  cur.Threshold == want.Threshold &&
		   cur.Shares == want.Shares
   }

// CerDriftProof 决策与 Exec 夹缝的不可变拒执行证明。
type CerDriftProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"decide_lease_id"`
	BatchID string `json:"batch_id"`
	Kind         ProofKind `json:"kind"`
	QuotedPolicy string    `json:"quoted_policy_id"`
	LivePolicy string `json:"live_policy_id"`
	QuotedThresh int `json:"quoted_threshold"`
	LiveThresh int `json:"live_threshold"`
	QuotedShares int `json:"quoted_shares"`
	LiveShares  int    `json:"live_shares"`
	QuotedRisk  string `json:"quoted_dest_risk"`
	LiveRisk    string `json:"live_dest_risk"`
	QuotedForce bool   `json:"quoted_force_cold"`
	LiveForce    bool   `json:"live_force_cold"`
	QuotedGen    int64  `json:"quoted_generation"`
	LiveGen      int64  `json:"live_generation"`
	QuotedDigest string `json:"quoted_digest"`
	LiveDigest string `json:"live_digest"`
	Detail     string `json:"detail"`
	Actor string `json:"actor"`
	At time.Time `json:"at"`
	CreatedAt time.Time `json:"created_at"`
}

// Copy 证明值拷贝。

  func (p CerDriftProof) Copy() CerDriftProof { return p }

// DecideAudit 租约审计流水。
type DecideAudit struct {
	AuditID string `json:"audit_id"`
	LeaseID string `json:"decide_lease_id"`
	BatchID string `json:"batch_id"`
	Action  string `json:"action"`
	From string `json:"from_state"`
	To    string `json:"to_state"`
	Actor string `json:"actor"`
	Detail string `json:"detail"`
	At time.Time `json:"at"`
}

// Copy 审计值拷贝。

   func (a DecideAudit) Copy() DecideAudit { return a }

// DecideGeneration 策略包/风险档世代。翻转即 +1，旧租约世代失效。
type DecideGeneration struct {
	Scope     string    `json:"scope"`
	Epoch     int64     `json:"epoch"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FlipEvent 待泵送的翻转事件。
type FlipEvent struct {
	EventID string   `json:"event_id"`
	Scope   string   `json:"scope"`
	Kind    FlipKind `json:"kind"`
	PolicyID string `json:"policy_id"`
	OldForce bool   `json:"old_force_cold"`
	NewForce bool   `json:"new_force_cold"`
	OldThresh int `json:"old_threshold"`
	NewThresh int `json:"new_threshold"`
	OldShares int `json:"old_shares"`
	NewShares int    `json:"new_shares"`
	OldRisk   string `json:"old_dest_risk"`
	NewRisk   string `json:"new_dest_risk"`
	Epoch     int64  `json:"epoch"`
	At time.Time `json:"at"`
	Done bool `json:"done"`
}

// Copy 事件值拷贝。

   func (e FlipEvent) Copy() FlipEvent { return e }

// LeaseConfig TTL 与批次上限。

type LeaseConfig struct {
	DefaultTTL time.Duration
	MaxTTL     time.Duration
	MaxOpen    int
}

// Fill 缺省。
   func (c *LeaseConfig) Fill() {
	  if c.DefaultTTL <= 0 {
		   c.DefaultTTL = 15 * time.Minute
	   }
	   if c.MaxTTL <= 0 {
		   c.MaxTTL = 2 * time.Hour
	  }
	   if c.MaxOpen <= 0 {
		  c.MaxOpen = 64
	   }
   }

// WorkerConfig 扫描间隔。

type WorkerConfig struct {
	WorkerID           string
	ExpiryScanInterval time.Duration
	FlipPumpInterval   time.Duration
	RiskPumpInterval   time.Duration
	BatchSize          int
}

// Fill 缺省。
   func (c *WorkerConfig) Fill() {
	   if c.WorkerID == "" {
		   c.WorkerID = "cer-decide-gate.worker"
	   }
	   if c.ExpiryScanInterval <= 0 {
		  c.ExpiryScanInterval = 8 * time.Second
	   }
	   if c.FlipPumpInterval <= 0 {
		  c.FlipPumpInterval = 5 * time.Second
	  }
	  if c.RiskPumpInterval <= 0 {
		  c.RiskPumpInterval = 5 * time.Second
	   }
	   if c.BatchSize <= 0 {
		   c.BatchSize = 64
	  }
  }

// DecideReq DecideWithLease 入参。
type DecideReq struct {
	Actor string `json:"actor"`
	IdempotencyKey string    `json:"idempotency_key"`
	TTLSec         int64     `json:"ttl_sec"`
	Line           LineFact  `json:"line"`
	Pin            DecidePin `json:"pin"`
	PackDigest string `json:"pack_digest"`
	SnapDigest string `json:"snap_digest"`
}

// BindReq Exec 前 AssertLease。
type BindReq struct {
	LeaseID string `json:"decide_lease_id"`
	Actor string `json:"actor"`
	ExecRef string   `json:"exec_ref"`
	Line    LineFact `json:"line"`
}

// ConsumeReq Exec 成功后消费租约。
type ConsumeReq struct {
	LeaseID string `json:"decide_lease_id"`
	Actor string `json:"actor"`
	ExecRef string `json:"exec_ref"`
}

// RevokeReq 运维吊销。

type RevokeReq struct {
	LeaseID string `json:"decide_lease_id"`
	Actor   string `json:"actor"`
	Why     string `json:"why"`
}

// LiveView 活策略与风险复验视图。
type LiveView struct {
	Pin       DecidePin
	ForceCold bool
	DestRisk string
	PackDigest string
	SnapDigest string
	LineDigest string
	Generation int64
	Fingerprint PolicyFingerprint
}

// TickReport 一轮巡检计数。
type TickReport struct {
	Expired int `json:"expired"`
	Revoked int `json:"revoked"`
	Flips   int `json:"flips"`
	Risks   int `json:"risks"`
}
