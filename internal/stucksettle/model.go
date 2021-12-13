package stucksettle

import (
	"fmt"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// StuckGeneration — transfer 级回收效果世代
// ──────────────────────────────────────────────────────────────────────────────
// StuckGeneration 为每个 transfer_id 维护单调递增的回收世代号。
// 每次合法 RecoverStuck 开始时世代号 +1；旧世代号请求被拒，不得产生资金动作。
// 持有 current_gen 的 RecoverLease 才可进入 Acting；其他请求被挡为 DupRecoverCase。

type StuckGeneration struct {
	StuckGenID string `json:"stuck_gen_id"`
	TransferID string `json:"transfer_id"`
	CurrentGen int64 `json:"current_gen"` // 当前世代号（从 1 开始）
	LastActingAt time.Time `json:"last_acting_at,omitempty"`
	TerminatedAt time.Time `json:"terminated_at,omitempty"` // 世代关闭时间（Recovered 或 Failed）
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// RecoverLease — 单次回收互斥租约
// ──────────────────────────────────────────────────────────────────────────────
// RecoverLeaseState 租约生命周期。
// 合法转换：Open → Acting → Recovered / DupBlocked / Expired。
type RecoverLeaseState string

const (
	LeaseOpen      RecoverLeaseState = "open"      // 租约已创建，待激活
	LeaseActing    RecoverLeaseState = "acting"    // 正在执行回收动作，互斥持锁中
	LeaseRecovered RecoverLeaseState = "recovered" // 回收完成，效果已固化
	LeaseDupBlocked RecoverLeaseState = "dup_blocked" // 被双结检测拦截，资金动作已撤销
	LeaseExpired    RecoverLeaseState = "expired"     // 租约超时未完成，世代失效
)

// RecoverLease 单次 RecoverStuck 互斥租约。
// 同一 transfer_id 只能存在一个 Acting 租约；第二个进入 DupBlocked。
// Generation 必须与 StuckGeneration.CurrentGen 一致，否则拒绝激活。
type RecoverLease struct {
	LeaseID string `json:"lease_id"`
	TransferID string `json:"transfer_id"`
	Generation int64 `json:"generation"` // 绑定的世代号
	State     RecoverLeaseState `json:"state"`
	HolderKey string            `json:"holder_key"` // 幂等键（节点 ID + 尝试次数）
	OpenedAt  time.Time         `json:"opened_at"`
	ActingAt  time.Time         `json:"acting_at,omitempty"`
	RecoveredAt time.Time `json:"recovered_at,omitempty"`
	BlockedAt   time.Time `json:"blocked_at,omitempty"`
	ExpiredAt   time.Time `json:"expired_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at"` // TTL 截止（Gate 强制）
	EffectProofID string    `json:"effect_proof_id,omitempty"` // 关联效果证明
	BlockReason   string    `json:"block_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// IsTerminal 租约是否处于终态。

   func (r *RecoverLease) IsTerminal() bool {
	   return r.State == LeaseRecovered || r.State == LeaseDupBlocked || r.State == LeaseExpired
  }

// ──────────────────────────────────────────────────────────────────────────────
// DupRecoverCase — 双结回收案件
// ──────────────────────────────────────────────────────────────────────────────
// DupRecoverCaseState 双结案件状态机。
// 合法转换：Open → Proven → Closed。

type DupRecoverCaseState string

const (
	DupCaseOpen   DupRecoverCaseState = "open"   // 已发现重复回收意图
	DupCaseProven DupRecoverCaseState = "proven" // 已附上效果证明，等待双人确认
	DupCaseClosed DupRecoverCaseState = "closed" // 已双人确认关闭（终态）
)

// DupRecoverCase 重复回收案件记录。
// 由 Dup Recover Detector 扫描或 RecoverLease 竞态产出。
// 产出 dup_proof_id 供他包只读引用；不复用 单笔出金失败补偿。

type DupRecoverCase struct {
	DupCaseID    string `json:"dup_case_id"`
	TransferID   string `json:"transfer_id"`
	Generation   int64  `json:"generation"`     // 发现时的世代号
	FirstLeaseID string `json:"first_lease_id"` // 合法首次租约 ID
	DupLeaseID   string `json:"dup_lease_id"`   // 重复/被拦截租约 ID
	State DupRecoverCaseState `json:"state"`
	DupProofID    string `json:"dup_proof_id,omitempty"`    // 关联证明
	ConfirmedBy   string `json:"confirmed_by,omitempty"`    // 人工确认人 1
	CoConfirmedBy string `json:"co_confirmed_by,omitempty"` // 人工确认人 2
	DetectedAt time.Time `json:"detected_at"`
	ProvenAt   time.Time `json:"proven_at,omitempty"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`
	CloseNote string    `json:"close_note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTerminal 案件是否已终态。

   func (d *DupRecoverCase) IsTerminal() bool {
	   return d.State == DupCaseClosed
  }

// ──────────────────────────────────────────────────────────────────────────────
// EffectProof — 回收终态效果证明
// ──────────────────────────────────────────────────────────────────────────────
// EffectProof 回收终态证明（append-only）。
// 不变量：proof_id 唯一；Checksum = SHA-256(intent ⊕ action ⊕ ledger_ref)。
// 证明缺失时不得将 StuckCase 标记为 Recovered。
type EffectProof struct {
	EffectProofID string `json:"effect_proof_id"` // 全局唯一，供他包只读引用
	TransferID string `json:"transfer_id"`
	LeaseID    string `json:"lease_id"`
	Generation int64  `json:"generation"`
	Intent    string `json:"intent"`     // 意图描述（RecoverStuck + transfer_id + gen）
	Action    string `json:"action"`     // 实际动作（wait/fee_bump/recover/fail）
	LedgerRef string `json:"ledger_ref"` // 账本/出金只读引用（transaction id 或 broadcast id）
	Checksum string `json:"checksum"` // SHA-256(intent|action|ledger_ref)
	IssuedBy string `json:"issued_by"`
	IssuedAt time.Time `json:"issued_at"`
}

// VerifyChecksum 校验效果证明完整性。
  func (p *EffectProof) VerifyChecksum() bool {
	  if p == nil {
		   return false
	   }
	   want := effectChecksum(p.Intent, p.Action, p.LedgerRef)
	   return want == p.Checksum
  }

// ──────────────────────────────────────────────────────────────────────────────
// AdvanceLock — Advance↔Recover 互斥锁记录
// ──────────────────────────────────────────────────────────────────────────────
// AdvanceLockState Advance 互斥锁状态。
type AdvanceLockState string

const (
	ALockHeld AdvanceLockState = "held" // AdvanceStuck 正在持锁
	ALockReleased AdvanceLockState = "released" // 已释放（允许 Recover）
)

// AdvanceLock 记录 AdvanceStuck 持锁期间对 RecoverStuck 的封锁。
// Hold 态（StuckHold）时拒绝旁路 Recover；AdvanceLock 释放后才允许直接 Recover 请求。
type AdvanceLock struct {
	LockID string `json:"lock_id"`
	TransferID string           `json:"transfer_id"`
	State      AdvanceLockState `json:"state"`
	HeldAt time.Time `json:"held_at"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	HolderNode string    `json:"holder_node"` // 持锁节点标识
	ExpiresAt time.Time `json:"expires_at"` // 防止 advance 崩溃后永久持锁
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// 请求/响应结构
// ──────────────────────────────────────────────────────────────────────────────
// AcquireLeaseReq 申请回收租约请求。

type AcquireLeaseReq struct {
	TransferID string `json:"transfer_id"`
	HolderKey  string `json:"holder_key"` // 持有者幂等键
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// CommitRecoverReq 提交回收结果并固化证明。
type CommitRecoverReq struct {
	LeaseID string `json:"lease_id"`
	Action    string `json:"action"`     // 实际动作
	LedgerRef string `json:"ledger_ref"` // 账本引用
	IssuedBy string `json:"issued_by"`
}

// ConfirmDupCaseReq 双人确认关闭双结案件。
type ConfirmDupCaseReq struct {
	DupCaseID   string `json:"dup_case_id"`
	ConfirmedBy string `json:"confirmed_by"`
	CoConfirmedBy string `json:"co_confirmed_by"`
	Note string `json:"note,omitempty"`
}

// ProofExportReq 导出证明请求。
type ProofExportReq struct {
	TransferID string `json:"transfer_id,omitempty"`
	LeaseID    string `json:"lease_id,omitempty"`
}

// ProofExport 证明导出形态（供审计）。
type ProofExport struct {
	Proof EffectProof `json:"proof"`
	Lease RecoverLease `json:"lease"`
	Generation StuckGeneration `json:"generation"`
	ExportedAt time.Time       `json:"exported_at"`
}

// GenMismatchErr 世代不匹配错误。

type GenMismatchErr struct {
	TransferID string
	Expected int64
	Got      int64
}

   func (e *GenMismatchErr) Error() string {
	  return fmt.Sprintf("stucksettle: generation mismatch transfer=%s expected=%d got=%d",
		   e.TransferID, e.Expected, e.Got)
   }

// CopyLease 浅拷贝防止外部修改内部状态。

  func CopyLease(l RecoverLease) RecoverLease { return l }

// CopyCase 浅拷贝。
  func CopyCase(c DupRecoverCase) DupRecoverCase { return c }

// CopyProof 浅拷贝。

  func CopyProof(p EffectProof) EffectProof { return p }

// CopyGen 浅拷贝。

   func CopyGen(g StuckGeneration) StuckGeneration { return g }
