package wdchalgate

import (
	"fmt"
	"time"
)

// ─── ChallengeLease 挑战租约 ──────────────────────────────────────────────
// LeaseState 挑战租约状态。
// 状态机：
// issued ──(CheckWithdraw 首次携带)──▶ presented
// presented ──(核验通过，消费完成)──▶ consumed [终态]
// issued | presented ──(TTL 超限，LeaseExpiryWorker)──▶ expired [终态]
// issued | presented ──(因子/combo_gen 漂移)──▶ stale [终态]
// issued | presented ──(手动吊销或 FailLocked 联动)──▶ revoked [终态]
type LeaseState string

const (
	// LeaseStateIssued 已签发，等待首次提交核验。
	LeaseStateIssued LeaseState = "issued"
	// LeaseStatePresented 已提交核验（首次），等待最终消费。
	LeaseStatePresented LeaseState = "presented"
	// LeaseStateConsumed 已消费（CheckWithdraw 核验通过并放行）。终态。
	LeaseStateConsumed LeaseState = "consumed"
	// LeaseStateExpired 已过期（TTL 超限）。终态。
	LeaseStateExpired LeaseState = "expired"
	// LeaseStateStale 已失效（因子/组合世代漂移）。终态。
	LeaseStateStale LeaseState = "stale"
	// LeaseStateRevoked 已吊销（手动或 FailLocked 联动）。终态。
	LeaseStateRevoked LeaseState = "revoked"
)

// Closed 判断租约是否为终态（不可再转换）。
   func (s LeaseState) Closed() bool {
	   switch s {
	  case LeaseStateConsumed, LeaseStateExpired, LeaseStateStale, LeaseStateRevoked:
		   return true
	   }
	  return false
   }

// ValidLeaseTransitions 合法租约状态转换表。

var ValidLeaseTransitions = map[LeaseState][]LeaseState{
	LeaseStateIssued:    {LeaseStatePresented, LeaseStateConsumed, LeaseStateExpired, LeaseStateStale, LeaseStateRevoked},
	LeaseStatePresented: {LeaseStateConsumed, LeaseStateExpired, LeaseStateStale, LeaseStateRevoked},
	LeaseStateConsumed: {},
	LeaseStateExpired: {},
	LeaseStateStale:   {},
	LeaseStateRevoked: {},
}

// CanTransit 校验状态转换是否合法。
  func CanTransit(from, to LeaseState) bool {
	  for _, t := range ValidLeaseTransitions[from] {
		   if t == to {
			   return true
		  }
	  }
	  return false
   }

// ChallengeLease 出金多因子挑战租约。
// 不变量：
// - combo_gen 签发时固化，CheckWithdraw 时与当前最大 gen ；gen 漂移则置 stale。
// - factor_sig 签发时固化（TOTP secret + combo 规则哈希）；CheckWithdraw 时复验。
// - dest_chain / dest_addr 可选绑定；非空时 CheckWithdraw 强制，地址不符拒绝。
// - 终态不可逆：consumed / expired / stale / revoked 一旦写入，Engine 禁止再次转换。
type ChallengeLease struct {
	// ID 租约唯一标识（UUID v4），对外称 chal_lease_id。
	ID string
	// UserID 发起用户 ID。
	UserID string
	// Factor 签发因子（totp / email / sms）。
	Factor string
	// ComboGen 签发时的因子组合世代。
	ComboGen int64
	// FactorSig 签发时因子绑定指纹（SHA-256 hex）。
	FactorSig string
	// State 当前状态。
	State LeaseState
	// DestChain 绑定目标链（可选）。
	DestChain string
	// DestAddr 绑定目标地址（可选）。
	DestAddr string
	// IdemKey 幂等键，防重复签发。
	IdemKey string
	// IssuedAt 签发时刻。
	IssuedAt time.Time
	// ExpiresAt 过期时刻。
	ExpiresAt time.Time
	// PresentedAt 首次提交核验时刻。
	PresentedAt *time.Time
	// ConsumedAt 消费完成时刻。
	ConsumedAt *time.Time
	// StaleAt 置 stale 时刻。
	StaleAt *time.Time
	// RevokeReason 吊销/stale 原因。
	RevokeReason string
	// ProofID 关联的 StaleChalProof.ID（stale 路径写入）。
	ProofID string
	// CreatedAt 行创建时刻。
	CreatedAt time.Time
	// UpdatedAt 行更新时刻。
	UpdatedAt time.Time
}

// Expired 判断租约是否已超 TTL（不含状态位判断，由 Engine 综合判断）。

  func (l *ChallengeLease) Expired(now time.Time) bool {
	   return now.After(l.ExpiresAt)
   }

// ─── ComboGeneration 因子组合世代 ─────────────────────────────────────────
// ChangeReason 世代变更原因。
type ChangeReason string

const (
	// ChangeReasonTOTPRebind TOTP 重绑定（BindTOTP 路径）。
	ChangeReasonTOTPRebind ChangeReason = "totp_rebind"
	// ChangeReasonComboReplace 组核验则变更（SetCombos 路径）。
	ChangeReasonComboReplace ChangeReason = "combo_replace"
	// ChangeReasonCooldownChange 冷却参数变更（外部只读观测触发）。
	ChangeReasonCooldownChange ChangeReason = "cooldown_change"
	// ChangeReasonFailLockUnlock FailLocked 解锁后主动推世代（防旧租约复活）。
	ChangeReasonFailLockUnlock ChangeReason = "faillock_unlock"
)

// ComboGeneration 因子组合世代变更记录（只追加，不修改）。
// 不变量：
// - gen 在用户维度单调递增；新纪录 gen = max(user_id) + 1。
// - FactorSig 记录变更后因子绑定指纹，与 ChallengeLease.FactorSig 基准一致。
// - 禁止回退 gen 或修改历史记录（审计完整性）。

type ComboGeneration struct {
	// ID 记录唯一标识（UUID v4）。
	ID string
	// UserID 用户 ID。
	UserID string
	// Gen 世代编号（单调递增）。
	Gen int64
	// ChangeReason 变更原因。
	ChangeReason ChangeReason
	// FactorSig 变更后因子绑定指纹 SHA-256 hex。
	FactorSig string
	// Actor 发起变更的操作人/系统标识。
	Actor string
	// RefID 关联外部单据 ID（如 TOTP 绑定 ID；可为空）。
	RefID string
	// ChangedAt 世代变更时刻。
	ChangedAt time.Time
	// CreatedAt 行创建时刻。
	CreatedAt time.Time
}

// ─── StaleChalProof 旧令牌拒放行证明 ─────────────────────────────────────
// ProofState 证明状态。
// 状态机：
// open ──(StaleChalWorker 固化)──▶ proven
// proven ──(管理员确认)──▶ closed [终态]
// open ──(驳回，无需 proven)──▶ closed [终态]

type ProofState string

const (
	// ProofStateOpen 证明已开启，等待固化。
	ProofStateOpen ProofState = "open"
	// ProofStateProven 证明已固化，等待管理员确认。
	ProofStateProven ProofState = "proven"
	// ProofStateClosed 证明已关闭（管理员确认或驳回）。终态。
	ProofStateClosed ProofState = "closed"
)

// Closed 判断证明是否为终态。

  func (s ProofState) Closed() bool {
	  return s == ProofStateClosed
  }

// DriftReason 因子漂移原因（精确标记，供审计）。
type DriftReason string

const (
	// DriftReasonTOTPRebind TOTP Secret 已重绑定，旧 TOTP OTP 失效。
	DriftReasonTOTPRebind DriftReason = "totp_rebind"
	// DriftReasonComboGenDrift 因子组合世代抬升，旧租约世代落后。
	DriftReasonComboGenDrift DriftReason = "combo_gen_drift"
	// DriftReasonCooldownRaise 安全冷却区间抬升，旧挑战窗口失效。
	DriftReasonCooldownRaise DriftReason = "cooldown_raise"
	// DriftReasonFailLockRevoke FailLocked 期间吊销，解锁后不得复活。
	DriftReasonFailLockRevoke DriftReason = "faillock_revoke"
)

// StaleChalProof 旧令牌拒放行证明。
// 不变量：
// - open 状态下，同 (UserID, LeaseID) 的 CheckWithdraw 一律拒绝并返回 proof_id。
// - ProofHash 唯一：防重复固化同一证明；由 StaleChalWorker 计算写入。
// - 终态 closed 不可逆。
type StaleChalProof struct {
	// ID 证明唯一标识（UUID v4）。
	ID string
	// LeaseID 旧 ChallengeLease.ID。
	LeaseID string
	// UserID 受影响用户 ID。
	UserID string
	// Factor 旧租约因子。
	Factor string
	// OldComboGen 旧租约签发时的 combo_gen。
	OldComboGen int64
	// NewComboGen 发现漂移时的当前 combo_gen。
	NewComboGen int64
	// DriftReason 漂移原因。
	DriftReason DriftReason
	// State 当前状态。
	State ProofState
	// ProofHash SHA-256 hex 证明指纹（唯一）。
	ProofHash string
	// IdemKey 幂等键，防重复开证明。
	IdemKey string
	// OpenedAt 证明开启时刻。
	OpenedAt time.Time
	// ProvenAt 证明固化时刻（nil = 未固化）。
	ProvenAt *time.Time
	// ClosedAt 证明关闭时刻（nil = 未关闭）。
	ClosedAt *time.Time
	// ClosedBy 关闭操作人 ID。
	ClosedBy string
	// Notes 关闭备注。
	Notes string
	// CreatedAt 行创建时刻。
	CreatedAt time.Time
	// UpdatedAt 行更新时刻。
	UpdatedAt time.Time
}

// ─── 哨兵错误 ─────────────────────────────────────────────────────────────
// ErrDuplicate 主键或幂等键冲突（InsertXxx 遇到已存在记录）。
var ErrDuplicate = fmt.Errorf("wdchalgate duplicate record")

// ErrNotFound 记录不存在。
var ErrNotFound = fmt.Errorf("wdchalgate record not found")

// ErrLeaseExpired 租约已过期或超 TTL。

var ErrLeaseExpired = fmt.Errorf("wdchalgate challenge lease expired")

// ErrLeaseStale 租约因子/组合世代漂移失效。

var ErrLeaseStale = fmt.Errorf("wdchalgate challenge lease stale")

// ErrLeaseRevoked 租约已吊销（FailLocked 或手动）。

var ErrLeaseRevoked = fmt.Errorf("wdchalgate challenge lease revoked")

// ErrLeaseConsumed 租约已消费（不可重复使用）。
var ErrLeaseConsumed = fmt.Errorf("wdchalgate challenge lease already consumed")

// ErrNoLease CheckWithdraw 未携带 chal_lease_id。
var ErrNoLease = fmt.Errorf("wdchalgate chal_lease_id required")

// ErrComboGenDrift 因子组合世代漂移（签发时 gen 与当前 gen 不符）。

var ErrComboGenDrift = fmt.Errorf("wdchalgate combo generation drifted")

// ErrFactorSigMismatch 因子绑定指纹不匹配（TOTP 已重绑）。
var ErrFactorSigMismatch = fmt.Errorf("wdchalgate factor signature mismatch")

// ErrDestMismatch 目标地址/链与租约绑定不符。

var ErrDestMismatch = fmt.Errorf("wdchalgate destination mismatch")

// ErrFailLocked 用户处于失败锁定状态，禁止签发/核验。

var ErrFailLocked = fmt.Errorf("wdchalgate user fail-locked")

// ErrInvalidTransit 非法状态转换。

var ErrInvalidTransit = fmt.Errorf("wdchalgate invalid state transition")

// ErrProofBlocked 存在 open StaleChalProof，挡写 CheckWithdraw。

var ErrProofBlocked = fmt.Errorf("wdchalgate blocked by open stale proof")
