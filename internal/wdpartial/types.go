package wdpartial

import (
	"encoding/json"
	"time"
)

// ──────────────────────────────────────────────
// 历史分片模型（ShardHold / PartialFill）
// ──────────────────────────────────────────────
// HoldState 分片冻结占用状态。

type HoldState string

const (
	HoldOpen     HoldState = "open"     // 冻结中，等待广播
	HoldConsumed HoldState = "consumed" // 已成交并消耗
	HoldReleased HoldState = "released" // 失败后释放
	HoldExpired HoldState = "expired" // 超时后过期（由运维介入）
)

// FillState 部分成交会计状态。

type FillState string

const (
	FillPending   FillState = "pending"   // 等待链上确认
	FillConfirmed FillState = "confirmed" // 链上已确认
	FillVoided FillState = "voided" // 作废（广播失败后）
)

// ShardHold 分片资金冻结占用。
// 一个 ShardLeg 对应一个 ShardHold。

type ShardHold struct {
	ID string `json:"id"`
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string    `json:"amount"`
	State  HoldState `json:"state"`
	FillID string `json:"fill_id,omitempty"`
	ReleaseReason string `json:"release_reason,omitempty"`
	// GuardGenID skipBroadcasting 腿绑定的守卫世代 ID（非空表示 skip 腿）。
	GuardGenID string `json:"guard_gen_id,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PartialFill 部分成交会计分录。
// 记录分片链上成交事件，供整单汇总计算。

type PartialFill struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
	HoldID string `json:"hold_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"` // 成交额（可小于或等于 Hold.Amount）
	FeeAmount string `json:"fee_amount,omitempty"`
	TxHash string `json:"tx_hash,omitempty"`
	Chain string    `json:"chain"`
	State FillState `json:"state"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OpenHoldReq 开立 ShardHold 请求。

type OpenHoldReq struct {
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount    string `json:"amount"`
	ExpiryTTL int    `json:"expiry_ttl_sec,omitempty"` // 过期秒数，0=不设
	// SkipLeg true 时为 skipBroadcasting 腿，须提供 LiveHash 创建 BroadcastGuardGen。
	SkipLeg  bool   `json:"skip_leg,omitempty"`
	LiveHash string `json:"live_hash,omitempty"`
}

// ConsumeHoldReq 消耗 ShardHold（链上成交）请求（历史分片模型，由 HoldService 使用）。
type ConsumeHoldReq struct {
	HoldID string `json:"hold_id"`
	FillID string `json:"fill_id"`
	Amount string `json:"amount"` // 成交额
	Actor  string `json:"actor"`
}

// ReleaseHoldReq 释放 ShardHold（腿失败或剩余释放）请求。
type ReleaseHoldReq struct {
	HoldID string `json:"hold_id"`
	Reason string `json:"reason"`
	Actor string `json:"actor"`
}

// CreateFillReq 创建 PartialFill 请求。
type CreateFillReq struct {
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
	HoldID string `json:"hold_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount    string `json:"amount"`
	FeeAmount string `json:"fee_amount,omitempty"`
	TxHash    string `json:"tx_hash,omitempty"`
	Chain string `json:"chain"`
}

// ConfirmFillReq 确认 PartialFill（链上确认）请求（历史分片模型，由 HoldService 使用）。

type ConfirmFillReq struct {
	FillID string `json:"fill_id"`
	TxHash string `json:"tx_hash,omitempty"`
	Actor string `json:"actor"`
}

// PlanFillSummary 整单成交汇总（历史分片模型，由 HoldService 使用）。

type PlanFillSummary struct {
	PlanID    string `json:"plan_id"`
	TotalHeld string `json:"total_held"`
	TotalConsumed string `json:"total_consumed"`
	TotalReleased string `json:"total_released"`
	TotalOpen     string `json:"total_open"`
	FillCount     int    `json:"fill_count"`
	FillConfirmed int    `json:"fill_confirmed"`
	HasOpenConserveCase bool `json:"has_open_conserve_case,omitempty"`
}

  func (h *ShardHold) Copy() ShardHold {
	  if h == nil {
		   return ShardHold{}
	   }
	  return *h
   }

  func (f *PartialFill) Copy() PartialFill {
	  if f == nil {
		  return PartialFill{}
	   }
	  return *f
   }

// ──────────────────────────────────────────────
// 竞态收口模型（RemReleaseLease / FillMutexToken / ConserveCase / BroadcastGuardGen）
// ──────────────────────────────────────────────
// HoldStatus Hold 单状态（竞态收口模型）。

type HoldStatus string

const (
	HoldStatusOpen HoldStatus = "open"
	HoldStatusReleased HoldStatus = "released"
	HoldStatusConsumed HoldStatus = "consumed"
	HoldStatusVoid     HoldStatus = "void"
)

  func (s HoldStatus) IsTerminal() bool {
	  return s == HoldStatusReleased || s == HoldStatusConsumed || s == HoldStatusVoid
  }

// FillStatus Fill 记录状态（竞态收口模型）。
type FillStatus string

const (
	FillStatusPending   FillStatus = "pending"
	FillStatusConfirmed FillStatus = "confirmed"
	FillStatusRejected FillStatus = "rejected"
)

  func (s FillStatus) IsTerminal() bool {
	   return s == FillStatusConfirmed || s == FillStatusRejected
   }

// LeaseStatus 剩余释放租约状态。
type LeaseStatus string

const (
	LeaseStatusOpen LeaseStatus = "open"
	LeaseStatusReleasing LeaseStatus = "releasing"
	LeaseStatusBlocked   LeaseStatus = "blocked"
	LeaseStatusDone      LeaseStatus = "done"
	LeaseStatusRevoked LeaseStatus = "revoked"
)

   func (s LeaseStatus) IsTerminal() bool {
	  return s == LeaseStatusDone || s == LeaseStatusRevoked
  }

// ConserveCaseStatus 守恒案件状态。
type ConserveCaseStatus string

const (
	ConserveCaseOpen ConserveCaseStatus = "open"
	ConserveCaseProven ConserveCaseStatus = "proven"
	ConserveCaseClosed ConserveCaseStatus = "closed"
)

  func (s ConserveCaseStatus) IsTerminal() bool {
	  return s == ConserveCaseClosed
   }

// GuardGenStatus 广播守卫世代状态。
type GuardGenStatus string

const (
	GuardGenActive GuardGenStatus = "active"
	GuardGenInvalidated GuardGenStatus = "invalidated"
	GuardGenConsumed    GuardGenStatus = "consumed"
)

  func (s GuardGenStatus) IsTerminal() bool {
	   return s == GuardGenInvalidated || s == GuardGenConsumed
  }

// HoldRecord Hold 记录（竞态收口模型，由 Service 通过 HoldStore 管理）。

type HoldRecord struct {
	ID string `json:"id"`
	PlanID string `json:"plan_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Seq    int        `json:"seq"`
	Status HoldStatus `json:"status"`
	GuardGenID string     `json:"guard_gen_id,omitempty"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// FillRecord Fill 记录（竞态收口模型）。
type FillRecord struct {
	ID string `json:"id"`
	PlanID string `json:"plan_id"`
	HoldID string `json:"hold_id"`
	UserID string `json:"user_id"`
	Asset  string     `json:"asset"`
	Amount string     `json:"amount"`
	Seq    int        `json:"seq"`
	Status FillStatus `json:"status"`
	MutexTokenID string `json:"mutex_token_id,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RemPlanFillSummary 整单成交汇总（竞态收口模型，由 Service.GetSummary 返回）。

type RemPlanFillSummary struct {
	PlanID    string `json:"plan_id"`
	TotalHeld string `json:"total_held"`
	TotalFilled string `json:"total_filled"`
	Remainder           string    `json:"remainder"`
	HoldCount           int       `json:"hold_count"`
	FillCount           int       `json:"fill_count"`
	HasOpenConserveCase bool      `json:"has_open_conserve_case"`
	ComputedAt          time.Time `json:"computed_at"`
}

// RemReleaseLease 剩余释放租约。
type RemReleaseLease struct {
	ID     string `json:"rem_release_lease_id"`
	PlanID string `json:"plan_id"`
	UserID     string `json:"user_id"`
	Generation int64  `json:"rem_release_gen"`
	Status LeaseStatus `json:"status"`
	BlockedByFillID string `json:"blocked_by_fill_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	ReleasingAt *time.Time `json:"releasing_at,omitempty"`
	DoneAt      *time.Time `json:"done_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FillMutexToken Fill 互斥令牌（仅内存传递，禁止持久化）。

type FillMutexToken struct {
	ID     string `json:"fill_mutex_token_id"`
	PlanID string `json:"plan_id"`
	FillID string `json:"fill_id"`
	Generation int64 `json:"generation"`
	Token string `json:"-"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
}

// IsValid 检查令牌是否仍在有效期且未消费。
  func (t *FillMutexToken) IsValid(now time.Time) bool {
	  if t == nil {
		  return false
	  }
	  if t.ConsumedAt != nil {
		  return false
	   }
	  return now.Before(t.ExpiresAt)
   }

// ConserveCase 守恒案件（PlanConservation 失败后升级创建）。
type ConserveCase struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	UserID string             `json:"user_id"`
	Status ConserveCaseStatus `json:"status"`
	ConserveSnapshot string `json:"conserve_snapshot"`
	FailReason       string `json:"fail_reason"`
	ProofPayload     string `json:"proof_payload,omitempty"`
	Approver string `json:"approver,omitempty"`
	Approver2  string     `json:"approver2,omitempty"`
	DetectedAt time.Time  `json:"detected_at"`
	ProvenAt   *time.Time `json:"proven_at,omitempty"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// BroadcastGuardGen 广播守卫世代（skipBroadcasting 腿绑定）。

type BroadcastGuardGen struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	LiveHash string `json:"live_hash"`
	Generation        int64          `json:"generation"`
	Status            GuardGenStatus `json:"status"`
	InvalidatedReason string         `json:"invalidated_reason,omitempty"`
	InvalidatedAt *time.Time `json:"invalidated_at,omitempty"`
	ConsumedAt    *time.Time `json:"consumed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UnmarshalJSON 同时接受 leftover 夹具字段 guard_gen_id 与 store 字段 id。

   func (g *BroadcastGuardGen) UnmarshalJSON(data []byte) error {
	   type alias BroadcastGuardGen
	   aux := struct {
		   alias
		  GuardGenID string `json:"guard_gen_id"`
	  }{}
	  if err := json.Unmarshal(data, &aux); err != nil {
		   return err
	   }
	  *g = BroadcastGuardGen(aux.alias)
	   if g.ID == "" {
		   g.ID = aux.GuardGenID
	  }
	   return nil
   }

// HoldInput RegisterHold 请求（竞态收口模型）。
type HoldInput struct {
	PlanID string
	UserID string
	Asset  string
	Amount string
	Seq     int
	SkipLeg bool // 为 true 时须提供 liveHash，创建 BroadcastGuardGen 绑定
}

// FillInput RegisterFill 请求（竞态收口模型）。

type FillInput struct {
	PlanID string
	HoldID string
	UserID string
	Asset  string
	Amount string
	Seq int
}

// ReleasePlanRemainderReq 剩余释放请求（竞态收口模型，由 Service.ReleasePlanRemainder 使用）。
type ReleasePlanRemainderReq struct {
	PlanID     string
	LeaseID    string
	Generation int64
	Actor string
}

// ReleasePlanRemainderResp 剩余释放响应（竞态收口模型）。

type ReleasePlanRemainderResp struct {
	PlanID string `json:"plan_id"`
	LeaseID string `json:"lease_id"`
	ReleasedHolds []string `json:"released_holds"`
	Remainder     string   `json:"remainder"`
}

// RemConfirmFillReq 确认 Fill 请求（竞态收口模型，携带互斥令牌）。
type RemConfirmFillReq struct {
	PlanID string
	FillID string
	MutexToken string
	Generation int64
	Actor string
}

// RemConsumeHoldReq ConsumeHold 扣减请求（竞态收口模型，携带互斥令牌）。

type RemConsumeHoldReq struct {
	PlanID string
	HoldID string
	FillID string
	Amount     string
	MutexToken string
	Actor      string
}

// PlanConservationReq 守恒校验请求（竞态收口模型）。

type PlanConservationReq struct {
	PlanID string
	Actor  string
}

// PlanConservationResp 守恒校验响应（竞态收口模型）。

type PlanConservationResp struct {
	PlanID string `json:"plan_id"`
	Conserved bool `json:"conserved"`
	Summary RemPlanFillSummary `json:"summary"`
	CaseID string `json:"conserve_case_id,omitempty"`
}

// IssueLeaseReq 颁发 RemReleaseLease 请求。

type IssueLeaseReq struct {
	PlanID string
	UserID string
	TTL time.Duration
}

// IssueFillMutexReq 颁发 FillMutexToken 请求。
type IssueFillMutexReq struct {
	PlanID string
	FillID string
	TTL    time.Duration
}

// ProveConserveCaseReq 守恒案件证明固化请求。
type ProveConserveCaseReq struct {
	CaseID       string
	ProofPayload string
	Approver     string
	Approver2    string
}

// CloseConserveCaseReq 守恒案件收口请求。
type CloseConserveCaseReq struct {
	CaseID   string
	Approver string
}

// ForceReleaseReq 双人确认强制释放请求（绕过 ConserveCase 阻断）。

type ForceReleaseReq struct {
	PlanID string
	HoldID string
	CaseID string
	Approver  string
	Approver2 string
	Reason    string
}

// InvalidateGuardGenReq 使守卫世代失效请求。

type InvalidateGuardGenReq struct {
	GuardGenID string
	Reason     string
}
