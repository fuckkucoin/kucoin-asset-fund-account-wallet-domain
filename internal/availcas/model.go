package availcas

import (
	"time"
	"github.com/shopspring/decimal"
)

// ────────────────────────────────────────────────────────────────────────────
// LeaseState 租约状态枚举
// ────────────────────────────────────────────────────────────────────────────
// LeaseState 描述一个可用快照租约的生命周期相位。

type LeaseState string

const (
	// LeaseStateOpen 租约已颁发，尚未绑定任何意图。
	LeaseStateOpen LeaseState = "open"
	// LeaseStateBound 租约已绑定到一个已接受的意图，不可再绑定。
	LeaseStateBound LeaseState = "bound"
	// LeaseStateExpired 租约已超过 TTL 自然过期；需申请新租约。
	LeaseStateExpired LeaseState = "expired"
	// LeaseStateRevoked 余额已被外部（非意图）变更撤销；该窗口可用快照不再可信。
	LeaseStateRevoked LeaseState = "revoked"
)

// ────────────────────────────────────────────────────────────────────────────
// IntentState 意图状态枚举
// ────────────────────────────────────────────────────────────────────────────
// IntentState 描述一个扣减/冻结意图的生命周期相位。

type IntentState string

const (
	// IntentStateProposed 意图已提交，正等待屏障裁决。
	IntentStateProposed IntentState = "proposed"
	// IntentStateAccepted 意图通过 CAS 检验，屏障接受；可接着执行账本操作。
	IntentStateAccepted IntentState = "accepted"
	// IntentStateRejected 意图被 CAS 屏障拒绝，含拒绝码；需重试或放弃。
	IntentStateRejected IntentState = "rejected"
)

// ────────────────────────────────────────────────────────────────────────────
// AvailLease 可用快照租约
// ────────────────────────────────────────────────────────────────────────────
// AvailLease 对用户×资产×账本位出具一个短存续的可用余额快照租约。
// 租约携带世代号（Gen），意图提交时必须携带同一 Gen 才能通过 CAS 检验。
// 可用额计算公式（只读汇入，禁止写任何外部科目）：
// Available = Balance − Frozen − Reserved − InFlight
// 其中：
// - Balance : 账本科目总余额（ledger.Service 只读查询）
// - Frozen : holdcat 已冻额（出金/划转/风控 hold 之和）
// - Reserved : r3 热钱包预留金只读（不改写其语义）
// - InFlight : 本窗口已接受的意图扣减额之和

type AvailLease struct {
	// ID 全局唯一 UUID。
	ID string `json:"id"`
	// UserID 用户标识。
	UserID string `json:"user_id"`
	// Asset 资产代码（如 USDT、BTC）。
	Asset string `json:"asset"`
	// Book 账本位（如 main、earn、trade）。
	Book string `json:"book"`
	// Gen 世代号，单调递增；意图 CAS 必须携带此值。
	Gen int64 `json:"gen"`
	// Balance 快照时刻账本总余额。
	Balance decimal.Decimal `json:"balance"`
	// Frozen 快照时刻已冻额（holdcat 汇总）。
	Frozen decimal.Decimal `json:"frozen"`
	// Reserved 快照时刻热钱包预留金（只读）。
	Reserved decimal.Decimal `json:"reserved"`
	// InFlight 快照时刻已接受意图的待执行扣减总额。
	InFlight decimal.Decimal `json:"in_flight"`
	// Available 净可用余额 = Balance − Frozen − Reserved − InFlight。
	Available decimal.Decimal `json:"available"`
	// State 当前状态。
	State LeaseState `json:"state"`
	// ExpiresAt 过期时刻；超过后提交意图必须失败。
	ExpiresAt time.Time `json:"expires_at"`
	// CreatedAt 租约颁发时刻。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后状态变更时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

// Active 租约是否仍在有效期且未终态。

   func (l *AvailLease) Active(now time.Time) bool {
	  if l == nil {
		   return false
	  }
	  return (l.State == LeaseStateOpen || l.State == LeaseStateBound) &&
		   now.Before(l.ExpiresAt)
  }

// Usable 租约是否可以绑定新意图（open 且未过期）。

  func (l *AvailLease) Usable(now time.Time) bool {
	  if l == nil {
		   return false
	   }
	   return l.State == LeaseStateOpen && now.Before(l.ExpiresAt)
  }

// ────────────────────────────────────────────────────────────────────────────
// DebitIntent 扣减/冻结意图
// ────────────────────────────────────────────────────────────────────────────
// DebitIntent 代表一次扣减或冻结操作的意图。
// 提交时必须携带 LeaseID + LeaseGen + ExpectedAvail；
// CAS 屏障确认这三个字段与当前租约一致后方可 accepted。
type DebitIntent struct {
	// ID 全局唯一 UUID。
	ID string `json:"id"`
	// IdemKey 幂等键；相同 IdemKey 的意图仅接受一次，后续重复返回 ErrDuplicateIntent。
	IdemKey string `json:"idem_key"`
	// UserID 用户标识（必须与 Lease.UserID 一致）。
	UserID string `json:"user_id"`
	// Asset 资产代码（必须与 Lease.Asset 一致）。
	Asset string `json:"asset"`
	// Book 账本位（必须与 Lease.Book 一致）。
	Book string `json:"book"`
	// Amount 意图扣减/冻结额，必须 > 0。
	Amount decimal.Decimal `json:"amount"`
	// Kind 操作类型：debit（直接扣减）或 freeze（冻结占位）。
	Kind string `json:"kind"`
	// Reason 业务语义说明，如 "withdraw_freeze" / "transfer_debit"。
	Reason string `json:"reason"`
	// LeaseID 所持租约 ID。
	LeaseID string `json:"lease_id"`
	// LeaseGen 所持租约世代号；必须与当前 Lease.Gen 完全一致。
	LeaseGen int64 `json:"lease_gen"`
	// ExpectedAvail 提交时期望可用余额；必须 ≥ Amount 且与 Lease.Available 一致。
	ExpectedAvail decimal.Decimal `json:"expected_avail"`
	// State 当前状态。
	State IntentState `json:"state"`
	// RejectCode 若 rejected，记录具体拒绝原因码。
	RejectCode RejectCode `json:"reject_code,omitempty"`
	// RejectNote 拒绝说明（人工可读）。
	RejectNote string `json:"reject_note,omitempty"`
	// RaceProofID 若产生竞态证明，记录 proof ID（仅 rejected 意图填充）。
	RaceProofID string `json:"race_proof_id,omitempty"`
	// ActorID 发起方标识（出金服务、划转服务等）。
	ActorID string `json:"actor_id,omitempty"`
	// RequestIP 来源 IP（审计）。
	RequestIP string `json:"request_ip,omitempty"`
	// ProposedAt 意图提交时刻。
	ProposedAt time.Time `json:"proposed_at"`
	// DecidedAt 屏障裁决时刻。
	DecidedAt *time.Time `json:"decided_at,omitempty"`
}

// Finalized 意图已达终态（不可再变更）。
  func (d *DebitIntent) Finalized() bool {
	  if d == nil {
		   return false
	   }
	   return d.State == IntentStateAccepted || d.State == IntentStateRejected
   }

// IntentKind 意图操作类型。

const (
	IntentKindDebit = "debit"
	IntentKindFreeze = "freeze"
)

// ────────────────────────────────────────────────────────────────────────────
// RaceProof 竞态证明
// ────────────────────────────────────────────────────────────────────────────
// RaceProof 记录同一用户×资产×账本位在同一窗口内发生的 CAS 冲突证明。
// 证明包含两个意图（winner/loser）、竞争窗口、拒绝码，供 desk 查询和事故复盘。
// live-hash 场景：loser 拒绝不触发任何解冻操作；proof 仅作证据落仓。
type RaceProof struct {
	// ID 全局唯一 UUID。
	ID string `json:"id"`
	// UserID 竞态发生的用户。
	UserID string `json:"user_id"`
	// Asset 竞态资产代码。
	Asset string `json:"asset"`
	// Book 竞态账本位。
	Book string `json:"book"`
	// LeaseID 共享的租约 ID（winner/loser 来自同一租约窗口）。
	LeaseID string `json:"lease_id"`
	// WinnerIntentID 通过 CAS 的意图 ID（CAS 胜者）。
	WinnerIntentID string `json:"winner_intent_id"`
	// LoserIntentID 被 CAS 屏障拒绝的意图 ID（CAS 败者）。
	LoserIntentID string `json:"loser_intent_id"`
	// WinnerAmount 胜者意图金额。
	WinnerAmount decimal.Decimal `json:"winner_amount"`
	// LoserAmount 败者意图金额。
	LoserAmount decimal.Decimal `json:"loser_amount"`
	// AvailAtDecision CAS 裁决时刻的净可用余额快照。
	AvailAtDecision decimal.Decimal `json:"avail_at_decision"`
	// WindowStart 竞争窗口起始（租约 CreatedAt）。
	WindowStart time.Time `json:"window_start"`
	// WindowEnd 竞争窗口结束（拒绝裁决时刻）。
	WindowEnd time.Time `json:"window_end"`
	// RejectCode 败者被拒绝的原因码。
	RejectCode RejectCode `json:"reject_code"`
	// Note 补充说明。
	Note string `json:"note,omitempty"`
	// CreatedAt 证明落库时刻。
	CreatedAt time.Time `json:"created_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// BarrierDecision 屏障裁决结果
// ────────────────────────────────────────────────────────────────────────────
// BarrierDecision 描述一次 Barrier.Submit 调用的完整裁决结果。

type BarrierDecision struct {
	// Accepted 意图是否被接受。
	Accepted bool `json:"accepted"`
	// Intent 已裁决的意图（包含更新后的 State / RejectCode）。
	Intent *DebitIntent `json:"intent"`
	// Proof 若发生竞态，包含落库的 RaceProof；否则为 nil。
	Proof *RaceProof `json:"proof,omitempty"`
	// AvailAfter 裁决后的净可用余额（仅 accepted 时有效）。
	AvailAfter decimal.Decimal `json:"avail_after,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────
// BarrierMetrics 屏障运行指标快照（用于 /v1/avail-cas/metrics）
// ────────────────────────────────────────────────────────────────────────────
// BarrierMetrics 汇总屏障引擎的运行期计数。
type BarrierMetrics struct {
	// LeasesIssued 已颁发租约总数。
	LeasesIssued int64 `json:"leases_issued"`
	// LeasesExpired 自然过期租约数。
	LeasesExpired int64 `json:"leases_expired"`
	// LeasesRevoked 被撤销租约数。
	LeasesRevoked int64 `json:"leases_revoked"`
	// IntentsAccepted 已接受意图数。
	IntentsAccepted int64 `json:"intents_accepted"`
	// IntentsRejected 已拒绝意图数（含超提、CAS 冲突、过期）。
	IntentsRejected int64 `json:"intents_rejected"`
	// RaceProofsCreated 已创建竞态证明数。
	RaceProofsCreated int64 `json:"race_proofs_created"`
	// OverdrawAttempts 超提企图次数（ExpectedAvail < Amount）。
	OverdrawAttempts int64 `json:"overdraw_attempts"`
}
