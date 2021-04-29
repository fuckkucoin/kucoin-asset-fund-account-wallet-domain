package egressleasegate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

// ─── 租约状态机 ───────────────────────────────────────────────────────────
// LeaseState 出金门控租约状态。
// issued ──(BroadcastAssert 绑定)──▶ bound
// bound ──(复验通过)──▶ consumed
// issued | bound ──(TTL)──▶ expired
// issued | bound ──(Halt/水位/指纹漂移)──▶ revoked
type LeaseState string

const (
	LeaseStateIssued   LeaseState = "issued"
	LeaseStateBound    LeaseState = "bound"
	LeaseStateConsumed LeaseState = "consumed"
	LeaseStateExpired LeaseState = "expired"
	LeaseStateRevoked LeaseState = "revoked"
	LeaseIssued = LeaseStateIssued
	LeaseBound  = LeaseStateBound
	LeaseConsumed = LeaseStateConsumed
	LeaseExpired = LeaseStateExpired
	LeaseRevoked = LeaseStateRevoked
)

// Closed 终态不可再转换。

  func (s LeaseState) Closed() bool {
	   switch s {
	   case LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked:
		  return true
	   }
	  return false
  }

// Open 租约仍可被广播复验。
  func (s LeaseState) Open() bool {
	  return s == LeaseStateIssued || s == LeaseStateBound
  }

// ValidLeaseTransitions 合法转换表。
var ValidLeaseTransitions = map[LeaseState][]LeaseState{
	LeaseStateIssued: {LeaseStateBound, LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateBound: {LeaseStateConsumed,
		LeaseStateExpired, LeaseStateRevoked},
	LeaseStateConsumed: {},
	LeaseStateExpired:  {},
	LeaseStateRevoked:  {},
}

// CanTransit 校验状态转换。

  func CanTransit(from, to LeaseState) bool {
	   for _, t := range ValidLeaseTransitions[from] {
		  if t == to {
			  return true
		   }
	  }
	  return false
   }

// RevokeReason 吊销原因。
type RevokeReason string

const (
	RevokeHaltFlip     RevokeReason = "halt_flip"
	RevokeLevelHalt    RevokeReason = "level_halt"
	RevokeFlagsDrift   RevokeReason = "flags_drift"
	RevokeTankDrift    RevokeReason = "tank_drift"
	RevokeRefillActive RevokeReason = "refill_active"
	RevokeManual       RevokeReason = "manual"
	RevokeExpiry RevokeReason = "expiry"
	RevokeMissingLease RevokeReason = "missing_lease"
)

// EgressLease 出金门控租约。
// 不变量：
// - BreachLevel / FlagsFingerprint / TankDigest / HaltGeneration 签发后只读。
// - 终态 consumed / expired / revoked 不可逆。
// - 广播成功路径必须经过未过期且未吊销的租约。

type EgressLease struct {
	ID string
	Chain string
	Asset  string
	Native string
	Amount           string
	Decision         string
	DecisionAtIssue  floatdesk.InstantDecision
	BreachLevel      floatdesk.TankLevel
	FlagsFP          string
	FlagsFingerprint string
	TankDigest       string
	Generation       uint64
	HaltGeneration uint64
	HotAvailAtIssue string
	MinAtIssue string
	TargetAtIssue string
	Source        string
	State         LeaseState
	IdemKey      string
	BroadcastRef string
	BatchID string
	Actor   string
	IssuedAt  time.Time
	ExpiresAt time.Time
	BoundAt   *time.Time
	ConsumedAt *time.Time
	RevokedAt    *time.Time
	RevokeReason string
	DriftProofID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Expired 实时 TTL 判断。

   func (l *EgressLease) Expired(now time.Time) bool {
	   if l == nil {
		   return true
	  }
	   return now.After(l.ExpiresAt)
  }

// ScopeKey chain|asset。
   func (l *EgressLease) ScopeKey() string {
	   if l == nil {
		   return ""
	  }
	  return ScopeKey(l.Chain, l.Asset)
  }

// ScopeKey 链与资产组成的世代键。
  func ScopeKey(chain, asset string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

// ─── 漂移证明 ─────────────────────────────────────────────────────────────
// ProofState 证明状态。
type ProofState string

const (
	ProofStateOpen   ProofState = "open"
	ProofStateProven ProofState = "proven"
	ProofStateClosed ProofState = "closed"
)

// Closed 证明终态。
   func (s ProofState) Closed() bool {
	   return s == ProofStateClosed
  }

// DriftType 漂移类型。
type DriftType string

const (
	DriftHaltFlip  DriftType = "halt_flip"
	DriftLevelHalt DriftType = "level_halt"
	DriftFlags        DriftType = "flags_drift"
	DriftTank         DriftType = "tank_drift"
	DriftMissingLease DriftType = "missing_lease"
	DriftExpired      DriftType = "expired"
	DriftGen          DriftType = "generation_drift"
	DriftHaltAll   DriftType = "halt_all"
	DriftHaltChain DriftType = "halt_chain"
	DriftHaltAsset DriftType = "halt_asset"
	DriftWaterline   DriftType = "waterline"
	DriftTankMissing DriftType = "tank_missing"
	DriftBreach      DriftType = "breach"
	DriftTankDigest DriftType = "tank_digest"
	DriftMissing    DriftType = "missing_lease"
	DriftRefill     DriftType = "refill_active"
	DriftWarnPolicy DriftType = "warn_policy"
	DriftGeneration DriftType = "generation"
)

const (
	ProofOpen   = ProofStateOpen
	ProofProven = ProofStateProven
	ProofClosed = ProofStateClosed
)

// EgressDriftProof 拒广播漂移证明。一经写入内容字段不可改，仅状态可推进。
type EgressDriftProof struct {
	ID      string
	LeaseID string
	Chain string
	Asset     string
	DriftType DriftType
	OldFlagsFP string
	NewFlagsFP string
	OldFlagsFingerprint string
	NewFlagsFingerprint string
	OldBreach           string
	NewBreach           string
	OldTankDigest string
	NewTankDigest string
	OldGeneration     uint64
	NewGeneration     uint64
	OldHaltGeneration uint64
	NewHaltGeneration uint64
	BroadcastRef      string
	Detail            string
	State     ProofState
	ProofHash string
	IdemKey   string
	OpenedAt  time.Time
	ProvenAt  *time.Time
	ClosedAt *time.Time
	ClosedBy string
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Summary 证明可读摘要。

   func (p *EgressDriftProof) Summary() string {
	  if p == nil {
		  return ""
	  }
	   lease := p.LeaseID
	  if lease == "" {
		  lease = "(none)"
	  }
	  ref := p.BroadcastRef
	  if ref == "" {
		  ref = "(none)"
	   }
	   return fmt.Sprintf("egress lease %s reject broadcast %s type %s level %s->%s gen %d->%d",
		   lease, ref, p.DriftType, emptyDash(p.OldBreach), emptyDash(p.NewBreach), p.OldHaltGeneration, p.NewHaltGeneration)
   }

   func emptyDash(s string) string {
	   if s == "" {
		  return "-"
	   }
	   return s
   }

// ProofExportBody leftover 导出正文（与 ProofExport 并存）。
type ProofExportBody struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"egress_lease_id,omitempty"`
	Chain string `json:"chain"`
	Asset      string `json:"asset"`
	DriftType  string `json:"drift_type"`
	OldFlagsFP string `json:"old_flags_fp,omitempty"`
	NewFlagsFP string `json:"new_flags_fp,omitempty"`
	OldBreach  string `json:"old_breach,omitempty"`
	NewBreach  string `json:"new_breach,omitempty"`
	OldTankDigest string `json:"old_tank_digest,omitempty"`
	NewTankDigest string `json:"new_tank_digest,omitempty"`
	OldGeneration uint64 `json:"old_generation"`
	NewGeneration uint64 `json:"new_generation"`
	BroadcastRef  string `json:"broadcast_ref,omitempty"`
	State   string `json:"state"`
	Summary string `json:"summary"`
}

// Export leftover 导出。

   func (p *EgressDriftProof) Export() ProofExportBody {
	  if p == nil {
		  return ProofExportBody{}
	   }
	   return ProofExportBody{
		  ProofID: p.ID, LeaseID: p.LeaseID, Chain: p.Chain, Asset: p.Asset,
		   DriftType: string(p.DriftType), OldFlagsFP: p.OldFlagsFP, NewFlagsFP: p.NewFlagsFP,
		  OldBreach: p.OldBreach, NewBreach: p.NewBreach,
		  OldTankDigest: p.OldTankDigest,
		  NewTankDigest: p.NewTankDigest,
		   OldGeneration: p.OldHaltGeneration, NewGeneration: p.NewHaltGeneration,
		  BroadcastRef: p.BroadcastRef, State: string(p.State), Summary: p.Summary(),
	  }
   }

// ProofIdem leftover 幂等键。
  func ProofIdem(leaseID string, dt DriftType, broadcastRef, oldFP, newFP, oldBreach, newBreach string) string {
	  raw := strings.Join([]string{leaseID, string(dt), broadcastRef, oldFP, newFP, oldBreach, newBreach}, "|")
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
   }

// ─── 世代与审计 ───────────────────────────────────────────────────────────
// EgressGeneration 停仓或水位跌破推进的世代行。
type EgressGeneration struct {
	ID string
	Scope string
	ScopeKey string
	Chain string
	Asset      string
	Generation uint64
	FlagsFP string
	FlagsFingerprint string
	Breach     string
	TankDigest string
	Decision   string
	Reason     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LeaseAudit 租约状态迁移审计。只增不改。

type LeaseAudit struct {
	ID      string
	LeaseID string
	FromState string
	ToState   string
	Actor string
	Note   string
	Action string
	Detail string
	At time.Time
	CreatedAt time.Time
}

// LiveView 复验时活门控快照（不回写 floatdesk）。
type LiveView struct {
	Decision floatdesk.InstantDecision
	BreachLevel      floatdesk.TankLevel
	FlagsFingerprint string
	TankDigest       string
	HaltGeneration uint64
	HotAvail string
	Reason string
}

// ProofExport 证明导出包。
type ProofExport struct {
	Proof EgressDriftProof `json:"proof"`
	Lease      *EgressLease `json:"lease,omitempty"`
	ExportedAt time.Time    `json:"exported_at"`
	ExportedBy string `json:"exported_by"`
}

// ─── 哨兵错误 ─────────────────────────────────────────────────────────────

var (
	ErrDuplicate = fmt.Errorf("egressleasegate duplicate record")
	ErrNotFound  = fmt.Errorf("egressleasegate record not found")
	ErrLeaseExpired = fmt.Errorf("egressleasegate lease expired")
	ErrLeaseRevoked = fmt.Errorf("egressleasegate lease revoked")
	ErrLeaseConsumed = fmt.Errorf("egressleasegate lease already consumed")
	ErrNoLease         = fmt.Errorf("egressleasegate egress_lease_id required")
	ErrHaltBlocked     = fmt.Errorf("egressleasegate halt blocks egress")
	ErrLevelHalt       = fmt.Errorf("egressleasegate water level halt")
	ErrFlagsDrift      = fmt.Errorf("egressleasegate flags fingerprint drifted")
	ErrTankDrift       = fmt.Errorf("egressleasegate tank digest drifted")
	ErrGenerationDrift = fmt.Errorf("egressleasegate halt generation drifted")
	ErrScopeMismatch  = fmt.Errorf("egressleasegate scope mismatch")
	ErrInvalidTransit = fmt.Errorf("egressleasegate invalid state transition")
	ErrProofBlocked = fmt.Errorf("egressleasegate blocked by open drift proof")
	ErrBroadcastMismatch = fmt.Errorf("egressleasegate broadcast_ref mismatch")
	ErrCheckDenied    = fmt.Errorf("egressleasegate check denied")
	ErrWarnNotAllowed = fmt.Errorf("egressleasegate warn decision not allowed by policy")
	ErrRefillActive    = fmt.Errorf("egressleasegate refill in progress refuses new lease")
	ErrDeskUnavailable = fmt.Errorf("egressleasegate floatdesk unavailable")
	ErrUnavailable     = fmt.Errorf("egressleasegate unavailable")
	ErrDrift      = fmt.Errorf("egressleasegate gate drifted")
	ErrHalt       = fmt.Errorf("egressleasegate halted")
	ErrWarnDenied = fmt.Errorf("egressleasegate warn not admitted")
	ErrGateDenied    = fmt.Errorf("egressleasegate gate denied")
	ErrBatchRejected = fmt.Errorf("egressleasegate batch rejected")
	ErrAmount        = fmt.Errorf("egressleasegate amount invalid")
	ErrChain      = fmt.Errorf("egressleasegate chain invalid")
	ErrAsset      = fmt.Errorf("egressleasegate asset invalid")
	ErrProofState = fmt.Errorf("egressleasegate proof state refused")
	ErrBatchLimit = fmt.Errorf("egressleasegate batch limit exceeded")
)
