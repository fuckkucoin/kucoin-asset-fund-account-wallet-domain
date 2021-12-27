package suspgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// 常量与枚举
// ──────────────────────────────────────────────────────────────────────────────
// SuspendedState 暂停世代的 FSM 状态节点。
type SuspendedState string

const (
	// SuspActive 已暂停，等待四眼清障。
	SuspActive SuspendedState = "active"
	// SuspClearing 四眼审核进行中（一审已到位，等待二审）。
	SuspClearing SuspendedState = "clearing"
	// SuspCleared 四眼审核完成，世代已消费。
	SuspCleared SuspendedState = "cleared"
)

// LeaseState 清障放行租约 FSM 状态节点。

type LeaseState string

const (
	// LeaseIssued 已签发，等待出金写路径绑定。
	LeaseIssued LeaseState = "issued"
	// LeaseConsumed 出金写路径已绑定并提交，正常终态。
	LeaseConsumed LeaseState = "consumed"
	// LeaseExpired TTL 到期，由 worker 流转；旧 lease 不再可用。
	LeaseExpired LeaseState = "expired"
)

// ProofOutcome 挡写证明的阻断结果。

type ProofOutcome string

const (
	// OutcomeBlocked 出金写路径被挡写（fail-closed）。
	OutcomeBlocked ProofOutcome = "blocked"
	// OutcomeDoubleCleared 同一 gen 被第二次 FourEyesClear；第二次记为漂移。
	OutcomeDoubleCleared ProofOutcome = "double_cleared"
	// OutcomeMissedBlock 计数已归零但仍有未消费 gen，疑似漏拦（审计泵写入）。
	OutcomeMissedBlock ProofOutcome = "missed_block"
)

// ──────────────────────────────────────────────────────────────────────────────
// SuspenseGeneration — 暂停世代记录
// ──────────────────────────────────────────────────────────────────────────────
// SuspenseGeneration 记录 (user_id, asset) 维度的暂停世代状态。
// 功能：
// GenValue 从 1 起，每次 Suspend 调用必须原子递增；
// ClearActor1/ClearActor2 在四眼清障中填充；
// State 从 active→clearing→cleared 单向流转。
// 重点：
// 同一 (UserID, Asset, GenValue) 组合视为不可变标识；
// 禁止在 cleared 状态后再次修改 GenValue。
// 风险点：
// SQL 层须用 CAS（WHERE gen_value = ? AND state = 'active'）保护 State 流转；
// 并发 Suspend 与 Clear 只允许一方提交成功。
type SuspenseGeneration struct {
	// GenID 主键，由 genIDFor(userID, asset, genValue) 构造。
	GenID string `json:"gen_id"`
	// UserID 被暂停用户。
	UserID string `json:"user_id"`
	// Asset 被暂停资产（大写，仅 USDT/USDC）。
	Asset string `json:"asset"`
	// GenValue 世代号，从 1 起单调递增。
	GenValue int64 `json:"gen_value"`
	// CaseID 触发此次暂停的失配案件 ID。
	CaseID string `json:"case_id"`
	// SuspendedBy 发起暂停的操作员。
	SuspendedBy string `json:"suspended_by"`
	// State FSM 状态。
	State SuspendedState `json:"state"`
	// ClearActor1 四眼清障第一审核人（可空，表示未进入清障流程）。
	ClearActor1 string `json:"clear_actor_1,omitempty"`
	// ClearActor2 四眼清障第二审核人（可空）。
	ClearActor2 string `json:"clear_actor_2,omitempty"`
	// ClearNote 清障备注。
	ClearNote string `json:"clear_note,omitempty"`
	// SuspendedAt 暂停时刻（UTC）。
	SuspendedAt time.Time `json:"suspended_at"`
	// ClearedAt 清障完成时刻（UTC）；未清障时为零值。
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
	// UpdatedAt 最后更新时刻（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// GenIDFor 构造 SuspenseGeneration 主键，形态：suspgen:<userID>:<asset>:<genValue>。

  func GenIDFor(userID, asset string, genValue int64) string {
	  raw := fmt.Sprintf("suspgen:%s:%s:%d",
		  strings.TrimSpace(userID),
		  strings.ToUpper(strings.TrimSpace(asset)),
		  genValue,
	  )
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
  }

// GenCounterKey 返回世代计数器的持久化键，形态：suspgen:counter:<userID>:<asset>。
   func GenCounterKey(userID, asset string) string {
	  return fmt.Sprintf("suspgen:counter:%s:%s",
		  strings.TrimSpace(userID),
		  strings.ToUpper(strings.TrimSpace(asset)),
	  )
  }

// ──────────────────────────────────────────────────────────────────────────────
// GenCounter — 世代计数器
// ──────────────────────────────────────────────────────────────────────────────
// GenCounter 记录 (user_id, asset) 的最新世代号与活跃暂停计数。
// 功能：
// CurrentGen 为已签发的最大世代号，供 Suspend 读取后递增；
// ActiveCount 等同于 brmismatch.SuspenseAccount.Count，两者须保持一致；
// UnconsumedGens 为 active/clearing 状态的世代 ID 集合，供 WithdrawAssert 检查。
// 风险点：
// CurrentGen 与 ActiveCount 须在同一数据库行（单行 CAS），防止计数与世代号分裂。
type GenCounter struct {
	// CounterKey 主键，由 GenCounterKey(userID, asset) 构造。
	CounterKey string `json:"counter_key"`
	// UserID 被暂停用户。
	UserID string `json:"user_id"`
	// Asset 被暂停资产。
	Asset string `json:"asset"`
	// CurrentGen 当前已发布的最大世代号；0 表示从未暂停。
	CurrentGen int64 `json:"current_gen"`
	// ActiveCount 当前活跃（active/clearing）世代的数量。
	ActiveCount int `json:"active_count"`
	// UpdatedAt 最后更新时刻（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// ClearanceLease — 清障放行租约
// ──────────────────────────────────────────────────────────────────────────────
// ClearanceLease 由 FourEyesClear 成功后签发，绑定到 (user_id, asset, gen_value)。
// 功能：
// 出金写路径须携带 LeaseID；WithdrawAssert 验证 lease 有效并消费之。
// TTL 默认 5 分钟；超时前若未消费则 worker 流转到 expired。
// 重点：
// LeaseID 全局唯一（UUID v4）；禁止缩短或重用。
// 同一 (UserID, Asset, GenValue) 仅允许一个 issued 状态的 lease；
// Clear 幂等时须返回已有 lease（若仍在 issued 状态）。
// 风险点：
// issued→consumed 须在出金写提交数据库成功后才调用 ConsumeLease；
// 提前消费会在写入失败后造成放行漏洞。
type ClearanceLease struct {
	// LeaseID 主键，UUID v4。
	LeaseID string `json:"lease_id"`
	// UserID 被清障用户。
	UserID string `json:"user_id"`
	// Asset 被清障资产。
	Asset string `json:"asset"`
	// GenValue 对应清障的世代号。
	GenValue int64 `json:"gen_value"`
	// CaseID 对应的失配案件 ID。
	CaseID string `json:"case_id"`
	// State 租约状态（issued / consumed / expired）。
	State LeaseState `json:"state"`
	// IssuedAt 签发时刻（UTC）。
	IssuedAt time.Time `json:"issued_at"`
	// ExpireAt 过期时刻（UTC）。
	ExpireAt time.Time `json:"expire_at"`
	// ConsumedAt 消费时刻（UTC）；未消费时为 nil。
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	// ConsumedBy 消费操作员（出金提交的 user 或系统标识）。
	ConsumedBy string `json:"consumed_by,omitempty"`
}

// IsValid 判断租约是否处于可消费的有效状态（issued 且未超时）。
   func (l *ClearanceLease) IsValid(now time.Time) bool {
	   return l != nil && l.State == LeaseIssued && now.Before(l.ExpireAt)
   }

// ──────────────────────────────────────────────────────────────────────────────
// SuspenseDriftProof — 挡写漂移证明
// ──────────────────────────────────────────────────────────────────────────────
// SuspenseDriftProof 只增审计记录，覆盖三类异常场景：
// 1. 出金写路径被挡写（OutcomeBlocked）；
// 2. 同一 gen 被第二次 FourEyesClear（OutcomeDoubleCleared）；
// 3. Count=0 但仍有未消费 gen 的漏拦（OutcomeMissedBlock）。
// 重点：
// ProofID 全局唯一（UUID v4）；禁止删改。
// CountBefore/CountAfter 记录操作前后的 ActiveCount，供双清复验。
// BlockOutcome 为枚举值，不得随意扩展。
// 风险点：
// SQL 层须用 INSERT IGNORE / ON CONFLICT DO NOTHING；禁止 UPDATE。
// proof_hash 由 (case_id + gen_value + outcome + attempted_at) 派生，防篡改。
type SuspenseDriftProof struct {
	// ProofID 主键，UUID v4。
	ProofID string `json:"proof_id"`
	// CaseID 关联失配案件 ID（可空，漏拦场景无关联案件时填 "none"）。
	CaseID string `json:"case_id"`
	// UserID 涉案用户。
	UserID string `json:"user_id"`
	// Asset 涉案资产。
	Asset string `json:"asset"`
	// SuspGen 事件发生时的世代号。
	SuspGen int64 `json:"susp_gen"`
	// CountBefore 操作前 ActiveCount。
	CountBefore int `json:"count_before"`
	// CountAfter 操作后 ActiveCount。
	CountAfter int `json:"count_after"`
	// BlockOutcome 阻断结果枚举。
	BlockOutcome ProofOutcome `json:"block_outcome"`
	// Detail 额外描述（可空）。
	Detail string `json:"detail,omitempty"`
	// ProofHash 防篡改哈希：sha256(case_id + "|" + gen + "|" + outcome + "|" + attempted_at.Unix)。
	ProofHash string `json:"proof_hash"`
	// AttemptedAt 事件发生时刻（UTC）。
	AttemptedAt time.Time `json:"attempted_at"`
}

// DriftProofHashFor 构造防篡改哈希。

  func DriftProofHashFor(caseID string, gen int64, outcome ProofOutcome, at time.Time) string {
	  raw := fmt.Sprintf("%s|%d|%s|%d", caseID, gen, outcome, at.UTC().Unix())
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
   }

// ──────────────────────────────────────────────────────────────────────────────
// 请求 / 结果结构体
// ──────────────────────────────────────────────────────────────────────────────
// SuspendWithGenReq 带世代的暂停请求。
type SuspendWithGenReq struct {
	// CaseID 触发暂停的失配案件 ID。
	CaseID string `json:"case_id"`
	// UserID 被暂停用户。
	UserID string `json:"user_id"`
	// Asset 被暂停资产（大写）。
	Asset string `json:"asset"`
	// Actor 操作员标识（非空）。
	Actor string `json:"actor"`
	// Note 操作备注（可空）。
	Note string `json:"note,omitempty"`
}

// SuspendWithGenResult 暂停操作结果。

type SuspendWithGenResult struct {
	// Gen 新签发的暂停世代记录。
	Gen SuspenseGeneration `json:"gen"`
	// Counter 更新后的世代计数器。
	Counter GenCounter `json:"counter"`
}

// ClearWithGenReq 带世代的四眼清障请求。

type ClearWithGenReq struct {
	// CaseID 要清障的失配案件 ID。
	CaseID string `json:"case_id"`
	// UserID 被清障用户。
	UserID string `json:"user_id"`
	// Asset 被清障资产。
	Asset string `json:"asset"`
	// GenValue 要消费的世代号（调用方须提供当前 active gen）。
	GenValue int64 `json:"gen_value"`
	// Actor1 第一审核人。
	Actor1 string `json:"actor_1"`
	// Actor2 第二审核人（须不同于 Actor1）。
	Actor2 string `json:"actor_2"`
	// Note 清障备注（可空）。
	Note string `json:"note,omitempty"`
}

// ClearWithGenResult 四眼清障结果。
type ClearWithGenResult struct {
	// Gen 清障后的世代记录。
	Gen SuspenseGeneration `json:"gen"`
	// Lease 签发的放行租约（若 gen 为本次清障的最后一个，则发放）。
	Lease *ClearanceLease `json:"lease,omitempty"`
	// DriftProof 若本次为重复清障，则落漂移证明（非空表示本次为幂等/漂移）。
	DriftProof *SuspenseDriftProof `json:"drift_proof,omitempty"`
}

// WithdrawAssertReq 出金挡写断言请求。
type WithdrawAssertReq struct {
	// UserID 出金用户。
	UserID string `json:"user_id"`
	// Asset 出金资产。
	Asset string `json:"asset"`
	// LeaseID 出金提交携带的 clearance_lease_id（可空；空表示无 lease）。
	LeaseID string `json:"lease_id,omitempty"`
	// RequireLease 若为 true，则即使 ActiveCount==0 也要求携带有效 lease。
	RequireLease bool `json:"require_lease"`
}

// WithdrawAssertResult 出金断言结果。
type WithdrawAssertResult struct {
	// Allowed 是否允许出金写入。
	Allowed bool `json:"allowed"`
	// BlockReason 拒绝原因（允许时为空）。
	BlockReason string `json:"block_reason,omitempty"`
	// ActiveCount 当前活跃暂停计数。
	ActiveCount int `json:"active_count"`
	// ConsumedLeaseID 若携带有效 lease 且已消费，填写 lease_id。
	ConsumedLeaseID string `json:"consumed_lease_id,omitempty"`
}
