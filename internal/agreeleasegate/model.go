package agreeleasegate

import (
	"fmt"
	"strings"
	"time"
)

// ─── 租约状态机 ───────────────────────────────────────────────────────────
// LeaseState 同意租约状态。
// agreed ──(PlanAssert)──▶ planned
// planned ──(ExecAssert)──▶ executing
// executing ──(末腿消费)──▶ consumed
// agreed | planned | executing ──(TTL)──▶ expired
// agreed | planned | executing ──(bal/rule 漂移)──▶ revoked
type LeaseState string

const (
	LeaseStateAgreed  LeaseState = "agreed"
	LeaseStatePlanned LeaseState = "planned"
	LeaseStateExecuting LeaseState = "executing"
	LeaseStateConsumed  LeaseState = "consumed"
	LeaseStateExpired   LeaseState = "expired"
	LeaseStateRevoked   LeaseState = "revoked"
)

// Closed 终态不可再转换。
  func (s LeaseState) Closed() bool {
	   switch s {
	  case LeaseStateConsumed,
		  LeaseStateExpired, LeaseStateRevoked:
		  return true
	  }
	  return false
   }

// Active 可进入 Plan/Exec 的非终态。

   func (s LeaseState) Active() bool {
	  switch s {
	  case LeaseStateAgreed, LeaseStatePlanned, LeaseStateExecuting:
		   return true
	  }
	  return false
  }

// ValidLeaseTransitions 合法转换表。

var ValidLeaseTransitions = map[LeaseState][]LeaseState{
	LeaseStateAgreed:    {LeaseStatePlanned, LeaseStateExpired, LeaseStateRevoked},
	LeaseStatePlanned:   {LeaseStateExecuting, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateExecuting: {LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateConsumed: {},
	LeaseStateExpired: {},
	LeaseStateRevoked: {},
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
	RevokeBalDrift RevokeReason = "bal_drift"
	RevokeRuleFlip  RevokeReason = "rule_flip"
	RevokeWaterline RevokeReason = "waterline"
	RevokeManual RevokeReason = "manual"
	RevokeExpiry       RevokeReason = "expiry"
	RevokeMissingLease RevokeReason = "missing_lease"
	RevokeAgreeFailed RevokeReason = "agree_failed"
	RevokeHalfPlan RevokeReason = "half_plan"
	RevokeKindsDrift RevokeReason = "kinds_drift"
)

// AgreeLease 归集读闸同意租约。
// 不变量：
// - BalFingerprint / RuleFingerprint / RuleVersion / Kinds 签发后只读。
// - 终态 consumed / expired / revoked 不可逆。
// - Plan 与 Exec 成功路径必须经过未过期且未吊销的租约。
// - 禁止双花：同一租约不得被两个 plan_ref 消费。

type AgreeLease struct {
	ID string
	Chain     string
	Asset     string
	AgreeCode string
	BalFingerprint  string
	RuleFingerprint string
	Kinds       string
	RuleVersion uint64
	ShadowBalAtIssue string
	WaterlineAtIssue string
	MinSweepAtIssue string
	State   LeaseState
	IdemKey string
	PlanRef  string
	ExecRef  string
	IssuedAt time.Time
	ExpiresAt   time.Time
	PlannedAt   *time.Time
	ExecutingAt *time.Time
	ConsumedAt   *time.Time
	RevokedAt    *time.Time
	RevokeReason string
	DriftProofID string
	CreatedAt    time.Time
	UpdatedAt time.Time
}

// Expired 实时 TTL 判断。
  func (l *AgreeLease) Expired(now time.Time) bool {
	   if l == nil {
		   return true
	  }
	  return now.After(l.ExpiresAt)
   }

// ScopeKey chain|asset。

   func (l *AgreeLease) ScopeKey() string {
	   if l == nil {
		   return ""
	  }
	   return l.Chain + "|" + l.Asset
   }

// KindSet 解析 kinds。

  func (l *AgreeLease) KindSet() []string {
	   if l == nil || strings.TrimSpace(l.Kinds) == "" {
		   return nil
	  }
	   parts := strings.Split(l.Kinds, ",")
	   out := make([]string,
		   0, len(parts))
	  for _, p := range parts {
		  p = strings.TrimSpace(p)
		   if p != "" {
			   out = append(out, p)
		  }
	   }
	   return out
  }

// ─── 指纹 ────────────────────────────────────────────────────────────────
// BalRuleFingerprint 余额与规则联合指纹。签发后只读。

type BalRuleFingerprint struct {
	Asset           string
	BalFingerprint  string
	RuleFingerprint string
	RuleVersion     uint64
	Kinds     string
	ShadowBal string
	Waterline string
}

// Combined 联合哈希输入串。

   func (f BalRuleFingerprint) Combined() string {
	   return f.Asset + "|" + f.BalFingerprint + "|" + f.RuleFingerprint + "|" +
		   fmt.Sprintf("%d", f.RuleVersion) + "|" + f.Kinds
  }

// Equal 精确比较。

   func (f BalRuleFingerprint) Equal(o BalRuleFingerprint) bool {
	   return f.Asset == o.Asset &&
		  f.BalFingerprint == o.BalFingerprint &&
		  f.RuleFingerprint == o.RuleFingerprint &&
		  f.RuleVersion == o.RuleVersion &&
		   f.Kinds == o.Kinds
  }

// FromLease 从租约提取。

   func FromLease(l *AgreeLease) BalRuleFingerprint {
	  if l == nil {
		   return BalRuleFingerprint{}
	   }
	   return BalRuleFingerprint{
		  Asset: l.Asset, BalFingerprint: l.BalFingerprint, RuleFingerprint: l.RuleFingerprint,
		   RuleVersion: l.RuleVersion,
		  Kinds:       l.Kinds,
		  ShadowBal:   l.ShadowBalAtIssue, Waterline: l.WaterlineAtIssue,
	   }
  }

// ─── 漂移证明 ─────────────────────────────────────────────────────────────
// ProofState 证明状态。
type ProofState string

const (
	ProofStateOpen ProofState = "open"
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
	DriftBal          DriftType = "bal_drift"
	DriftRule         DriftType = "rule_flip"
	DriftWaterline    DriftType = "waterline"
	DriftMissingLease DriftType = "missing_lease"
	DriftExpired     DriftType = "expired"
	DriftHalfPlan    DriftType = "half_plan"
	DriftKinds       DriftType = "kinds_drift"
	DriftAgreeFailed DriftType = "agree_failed"
	DriftVersion DriftType = "version_drift"
)

// AgreeDriftProof 拒执行漂移证明。一经写入内容字段不可改，仅状态可推进。
type AgreeDriftProof struct {
	ID      string
	LeaseID string
	Chain   string
	Asset   string
	DriftType DriftType
	OldBalFingerprint string
	NewBalFingerprint  string
	OldRuleFingerprint string
	NewRuleFingerprint string
	OldRuleVersion uint64
	NewRuleVersion uint64
	OldShadowBal string
	NewShadowBal string
	OldWaterline string
	NewWaterline string
	State        ProofState
	ProofHash    string
	IdemKey      string
	OpenedAt time.Time
	ProvenAt *time.Time
	ClosedAt *time.Time
	ClosedBy  string
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ─── 世代、半截计划、审计 ─────────────────────────────────────────────────
// AgreeGeneration 规则或水位推进的世代行。
type AgreeGeneration struct {
	ID    string
	Scope string
	Chain string
	Asset string
	Generation uint64
	RuleFingerprint string
	Reason    string
	CreatedAt time.Time
}

// HalfPlanBlock 半截计划挡写。
type HalfPlanBlock struct {
	ID      string
	LeaseID string
	PlanRef string
	Asset   string
	Reason  string
	Blocked bool
	CreatedAt time.Time
}

// LeaseAudit 租约状态迁移审计。只增不改。
type LeaseAudit struct {
	ID      string
	LeaseID string
	FromState string
	ToState   string
	Actor     string
	Note      string
	CreatedAt time.Time
}

// LiveView 复验时活读闸快照（不回写 AgreeGate 簿记）。
type LiveView struct {
	AgreeCode string
	BalFingerprint  string
	RuleFingerprint string
	RuleVersion uint64
	ShadowBal string
	Waterline string
	Kinds     string
	Reason    string
}

// ProofExport 证明导出包。
type ProofExport struct {
	Proof AgreeDriftProof `json:"proof"`
	Lease *AgreeLease `json:"lease,omitempty"`
	ExportedAt time.Time `json:"exported_at"`
	ExportedBy string    `json:"exported_by"`
}

// ─── 哨兵错误 ─────────────────────────────────────────────────────────────

var (
	ErrDuplicate = fmt.Errorf("agreeleasegate duplicate record")
	ErrNotFound = fmt.Errorf("agreeleasegate record not found")
	ErrLeaseExpired = fmt.Errorf("agreeleasegate lease expired")
	ErrLeaseRevoked = fmt.Errorf("agreeleasegate lease revoked")
	ErrLeaseConsumed = fmt.Errorf("agreeleasegate lease already consumed")
	ErrNoLease       = fmt.Errorf("agreeleasegate agree_lease_id required")
	ErrBalDrift      = fmt.Errorf("agreeleasegate shadow balance fingerprint drifted")
	ErrRuleDrift      = fmt.Errorf("agreeleasegate rule fingerprint drifted")
	ErrVersionDrift   = fmt.Errorf("agreeleasegate rule version raised")
	ErrWaterlineDrift = fmt.Errorf("agreeleasegate waterline crossed")
	ErrKindsDrift = fmt.Errorf("agreeleasegate kinds drifted")
	ErrScopeMismatch  = fmt.Errorf("agreeleasegate scope mismatch")
	ErrInvalidTransit = fmt.Errorf("agreeleasegate invalid state transition")
	ErrProofBlocked   = fmt.Errorf("agreeleasegate blocked by open drift proof")
	ErrPlanMismatch   = fmt.Errorf("agreeleasegate plan_ref mismatch")
	ErrExecMismatch   = fmt.Errorf("agreeleasegate exec_ref mismatch")
	ErrAgreeDenied = fmt.Errorf("agreeleasegate agree code failed")
	ErrGateUnavailable = fmt.Errorf("agreeleasegate agree gate unavailable")
	ErrHalfPlanBlocked = fmt.Errorf("agreeleasegate half-plan write blocked")
	ErrDoubleBind      = fmt.Errorf("agreeleasegate lease already bound to another plan")
)
