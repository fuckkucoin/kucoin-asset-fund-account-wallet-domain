package reclaim

import (
	"strings"
	"time"
)

// ─── SLA 常量 ────────────────────────────────────────────────────────────────

const (
	// SLAOpenBreachDur open 状态超时时间：72 小时。
	SLAOpenBreachDur = 72 * time.Hour
	// SLAReserveBreachDur reserved 状态超时时间：168 小时（7 天）。
	SLAReserveBreachDur = 168 * time.Hour
)

// ─── 案件状态 ────────────────────────────────────────────────────────────────
// State ReclaimCase 的生命周期状态。

type State string

const (
	// StateOpen 案件已开立，等待运营/系统预占输出。
	StateOpen State = "open"
	// StateReserved 输出已预占，等待链上退款/归集操作完成。
	StateReserved State = "reserved"
	// StateReclaimed 回收成功，资金已退回或归入核验账户，终态。
	StateReclaimed State = "reclaimed"
	// StateCancelled 回收取消（用户撤单、重复归因成功等），终态。
	StateCancelled State = "cancelled"
	// StateEscalated 已升级至高级运营/核验部门，待进一步处理（非终态，可接着转移）。
	StateEscalated State = "escalated"
	// StateAutoCredited 同链正常充值路径已贷记成功，案件自动关闭，终态。
	StateAutoCredited State = "auto_credited"
)

// Terminal 是否为终态（不可接着操作）。
  func (s State) Terminal() bool {
	  switch s {
	  case StateReclaimed,
		   StateCancelled, StateAutoCredited:
		  return true
	   default:
		   return false
	  }
   }

// ParseState 解析状态字符串；非法值返回 ErrIllegalTransition。

  func ParseState(s string) (State, error) {
	   switch State(strings.TrimSpace(s)) {
	   case StateOpen, StateReserved, StateReclaimed, StateCancelled, StateEscalated, StateAutoCredited:
		  return State(s), nil
	  default:
		  return "", ErrIllegalTransition
	  }
  }

// ─── 案件事件 ────────────────────────────────────────────────────────────────
// Event ReclaimCase FSM 转移事件。
type Event string

const (
	// EventReserve 运营预占输出（open → reserved）。
	EventReserve Event = "reserve"
	// EventReclaim 回收操作完成（reserved/escalated → reclaimed）。
	EventReclaim Event = "reclaim"
	// EventCancel 取消回收（open/reserved/escalated → cancelled）。
	EventCancel Event = "cancel"
	// EventEscalate 人工升级（任意非终态 → escalated）。
	EventEscalate Event = "escalate"
	// EventAutoCredit 同链正常充值贷记成功（reserved → auto_credited）。
	EventAutoCredit Event = "auto_credit"
	// EventSLAOpenBreach SLA 超时：open 状态超过 72h（open → escalated）。
	EventSLAOpenBreach Event = "sla_open_breach"
	// EventSLAReserveBreach SLA 超时：reserved 状态超过 168h（reserved → escalated）。
	EventSLAReserveBreach Event = "sla_reserve_breach"
)

// ─── 回收类型 ────────────────────────────────────────────────────────────────
// ReclaimKind 回收原因类别。
type ReclaimKind string

const (
	// KindWrongChain 发错链：用户在非预期链上发送了资产。
	KindWrongChain ReclaimKind = "wrong_chain"
	// KindUnclaimed 未认领：链上存在充值输出但无法匹配到任何用户账户。
	KindUnclaimed ReclaimKind = "unclaimed"
	// KindMemoMismatch Memo/Tag 与地址不匹配，无法归因。
	KindMemoMismatch ReclaimKind = "memo_mismatch"
	// KindExpiredAddress 充值地址已过期或被回收，输出无人认领。
	KindExpiredAddress ReclaimKind = "expired_address"
)

// ParseReclaimKind 解析回收类型字符串；非法值返回 ErrIllegalTransition。

   func ParseReclaimKind(s string) (ReclaimKind, error) {
	   switch ReclaimKind(strings.TrimSpace(s)) {
	  case KindWrongChain, KindUnclaimed, KindMemoMismatch, KindExpiredAddress:
		   return ReclaimKind(s), nil
	   default:
		   return "", ErrIllegalTransition
	  }
   }

// ─── 主案件记录 ───────────────────────────────────────────────────────────────
// ReclaimCase 错链/未认领充值回收案件主记录。
// TxHash + Vout 唯一标识一笔链上 UTXO 或账户模型输出（账户模型 Vout=0）。
// 同一输出至多一个活跃（非终态）ReclaimCase，由 ReclaimOccupancy 在内存和 DB 双重保障。
type ReclaimCase struct {
	// 案件标识
	ID string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	// 链上输出
	TxHash string `json:"tx_hash"`
	Vout int `json:"vout"`
	Chain  string `json:"chain"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"` // 链上输出金额（shopspring decimal 字符串）
	// 回收类型
	Kind ReclaimKind `json:"kind"`
	// 关联用户（可选：unclaimed 场景可能为空）
	UserID string `json:"user_id,omitempty"`
	// 目标地址（退款目标，由用户/运营提供）
	RefundAddress string `json:"refund_address,omitempty"`
	// 运营备注
	Note     string `json:"note,omitempty"`
	OpenedBy string `json:"opened_by"`
	// 预占信息
	ReservedBy string `json:"reserved_by,omitempty"`
	ReservedAt *time.Time `json:"reserved_at,omitempty"`
	// 回收结果
	ReclaimedTxHash string `json:"reclaimed_tx_hash,omitempty"`
	ReclaimedBy string `json:"reclaimed_by,omitempty"`
	ReclaimedAt *time.Time `json:"reclaimed_at,omitempty"`
	// 取消信息
	CancelledBy string `json:"cancelled_by,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CancelReason string `json:"cancel_reason,omitempty"`
	// 升级信息
	EscalatedBy string     `json:"escalated_by,omitempty"`
	EscalatedAt *time.Time `json:"escalated_at,omitempty"`
	EscalateNote string `json:"escalate_note,omitempty"`
	// 自动贷记信息（auto_credited 终态）
	AutoCreditRef string `json:"auto_credit_ref,omitempty"` // 贷记成功的 transfer_id
	AutoCreditAt *time.Time `json:"auto_credit_at,omitempty"`
	// SLA 截止时间
	SLAOpenDeadline time.Time `json:"sla_open_deadline"`
	SLAReserveDeadline time.Time `json:"sla_reserve_deadline,omitempty"`
	// 生命周期
	State State `json:"state"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 返回浅拷贝（time.Time 为值类型，已安全）。
  func (c ReclaimCase) Copy() ReclaimCase { return c }

// OutputKey 返回唯一标识链上输出的键，形态 "<txhash>:<vout>"。
  func (c ReclaimCase) OutputKey() string {
	   return outputKey(c.TxHash, c.Vout)
   }

// outputKey 构造输出键。

  func outputKey(txHash string, vout int) string {
	  return txHash + ":" + itoa(vout)
  }

// itoa 简单整数转字符串（避免导入 strconv 仅为此）。
   func itoa(n int) string {
	  if n == 0 {
		  return "0"
	  }
	  neg := n < 0
	  if neg {
		  n = -n
	   }
	  buf := [20]byte{}
	  pos := len(buf)
	  for n > 0 {
		   pos--
		   buf[pos] = byte('0' + n%10)
		   n /= 10
	   }
	  if neg {
		   pos--
		   buf[pos] = '-'
	   }
	  return string(buf[pos:])
  }

// ─── 竞态事件 ─────────────────────────────────────────────────────────────────
// LateCreditRaceEvent 当充值 Observe/Credit 与 ReclaimOccupancy 发生竞态时生成。
// 消费方（deposit service 或事件总线）负责记录并通知运营。

type LateCreditRaceEvent struct {
	OutputKey string `json:"output_key"` // TxHash:Vout
	CaseID    string `json:"case_id"`
	Chain string `json:"chain"`
	Asset      string `json:"asset"`
	Amount     string `json:"amount"`
	TransferID string `json:"transfer_id"` // 被阻塞的充值 transfer_id
	Reason string    `json:"reason"` // "credit_blocked_by_reclaim" 等
	At     time.Time `json:"at"`
}

// ─── 案件时间轴事件 ───────────────────────────────────────────────────────────
// TimelineEvent 案件时间轴事件，只追加不删改。

type TimelineEvent struct {
	CaseID string `json:"case_id"`
	Seq    int    `json:"seq"`
	Kind  string `json:"kind"`
	Actor string `json:"actor"`
	FromState State `json:"from_state,omitempty"`
	ToState State `json:"to_state"`
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}

// ─── 请求类型 ─────────────────────────────────────────────────────────────────
// OpenReq 开立 ReclaimCase 请求。
type OpenReq struct {
	IdempotencyKey string `json:"idempotency_key"`
	TxHash         string `json:"tx_hash"`
	Vout           int    `json:"vout"`
	Chain string `json:"chain"`
	Asset  string      `json:"asset"`
	Amount string      `json:"amount"`
	Kind   ReclaimKind `json:"kind"`
	UserID string      `json:"user_id,omitempty"`
	RefundAddress string `json:"refund_address,omitempty"`
	OpenedBy      string `json:"opened_by"`
	Note string `json:"note,omitempty"`
}

// ReserveReq 预占输出请求。

type ReserveReq struct {
	CaseID     string `json:"case_id"`
	Actor      string `json:"actor"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// ReclaimReq 完成回收请求。

type ReclaimReq struct {
	CaseID          string `json:"case_id"`
	ReclaimedTxHash string `json:"reclaimed_tx_hash"`
	Actor      string `json:"actor"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// CancelReq 取消回收请求。

type CancelReq struct {
	CaseID string `json:"case_id"`
	Actor      string `json:"actor"`
	Reason     string `json:"reason"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// EscalateReq 人工升级请求。

type EscalateReq struct {
	CaseID     string `json:"case_id"`
	Actor      string `json:"actor"`
	Note       string `json:"note"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// AutoCreditReq 同链自动贷记成功通知请求。

type AutoCreditReq struct {
	CaseID     string `json:"case_id"`
	TransferID string `json:"transfer_id"` // 贷记成功的 transfer_id
	Actor      string `json:"actor"`
}

// ListReq 案件列表查询请求。
type ListReq struct {
	Chain string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
	State  string `json:"state,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Kind string `json:"kind,omitempty"`
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}
