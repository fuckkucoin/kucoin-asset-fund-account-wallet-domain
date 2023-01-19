package viptiergate

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// Tier 档位枚举
// ─────────────────────────────────────────────────────────────────────────────
// Tier 档位值与 vipfee.Tier 保持一致（避免 import cycle，此处重声明）。
type Tier string

const (
	TierKYC0 Tier = "kyc0"
	TierStandard Tier = "standard"
	TierVIP     Tier = "vip"
	TierVIPPlus Tier = "vip+"
)

// TierRank 返回档位的数字排位；用于判断抬档/降档方向。
// rank 越大档位越高。
  func TierRank(t Tier) int {
	  switch t {
	  case TierKYC0:
		   return 0
	  case TierStandard:
		  return 1
	  case TierVIP:
		  return 2
	   case TierVIPPlus:
		   return 3
	   default:
		  return -1
	  }
   }

// IsUpgrade 判断从 from → to 是否为抬档。

   func IsUpgrade(from, to Tier) bool {
	   return TierRank(to) > TierRank(from)
  }

// IsDowngrade 判断从 from → to 是否为降档。
  func IsDowngrade(from, to Tier) bool {
	  return TierRank(to) < TierRank(from)
   }

// ─────────────────────────────────────────────────────────────────────────────
// TierGeneration
// ─────────────────────────────────────────────────────────────────────────────
// TierGeneration 记录用户（跨资产全局）的档位世代计数器。
// 世代协议（CAS）：
// Acquire:
// UPDATE vip_tier_generations
// SET generation = generation + 1, last_tier = ?, updated_at = NOW(3)
// WHERE user_id = ? AND generation = ?
// -- 影响行数 = 0 → ErrStaleGen；= 1 → 成功
// QuoteWithdraw 颁发世代令牌时快照当前 generation；
// 结算时再次读取并，不等则 ErrQuoteGenMismatch。

type TierGeneration struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Generation int64 `json:"generation"`
	LastTier Tier `json:"last_tier"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus — TierLease FSM 状态机枚举
// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus 表示 TierLease 的所有合法状态。

type LeaseStatus string

const (
	// LeaseOpen 初始态：档位切换请求已登记，等待在途冻额/Quote 检查通过。
	LeaseOpen LeaseStatus = "open"
	// LeaseBound 绑定态：切换操作已与新档位 Generation 绑定，等待结算确认。
	LeaseBound LeaseStatus = "bound"
	// LeaseSwitched 终态（成功）：档位切换已完成，所有在途报价均已按新档重算或过期。
	LeaseSwitched LeaseStatus = "switched"
	// LeaseBlocked 阻塞态：存在在途冻额或未结 Quote，切换暂被挂起进入待切换队列。
	// 后台 worker 定期扫描并在冻额/Quote 清零后推进至 LeaseBound。
	LeaseBlocked LeaseStatus = "blocked"
)

// ─────────────────────────────────────────────────────────────────────────────
// TierLease
// ─────────────────────────────────────────────────────────────────────────────
// TierLease 档位切换租约，记录一次 Pin/ClearPin/RecordVolume 触发的档位变更的完整生命周期。
// 状态机：
// open ──→ bound ──→ switched (正常路径)
// open ──→ blocked (在途冻额/Quote 互斥)
// blocked ──→ bound ──→ switched (待切换队列推进)
// open/bound ──→ blocked (检查补录发现新冻额)
// CAS 协议见 TierGeneration。

type TierLease struct {
	ID     string `json:"lease_id"`
	UserID string `json:"user_id"`
	FromTier    Tier   `json:"from_tier"`
	ToTier      Tier   `json:"to_tier"`
	Generation  int64  `json:"generation"`
	TriggerKind string `json:"trigger_kind"` // "pin" | "clear_pin" | "volume"
	TriggerRef  string `json:"trigger_ref"`  // Pin/Fill ID
	Status LeaseStatus `json:"status"`
	BlockReason string `json:"block_reason,omitempty"`
	BoundAt    *time.Time `json:"bound_at,omitempty"`
	SwitchedAt *time.Time `json:"switched_at,omitempty"`
	BlockedAt  *time.Time `json:"blocked_at,omitempty"`
	DriftCaseID string    `json:"drift_case_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

   func (l TierLease) Copy() TierLease { return l }

// IsTerminal 返回 true 若租约已达终态（switched 或被漂移封堵后关闭）。

  func (l TierLease) IsTerminal() bool {
	  return l.Status == LeaseSwitched
  }

// ─────────────────────────────────────────────────────────────────────────────
// QuoteBind
// ─────────────────────────────────────────────────────────────────────────────
// QuoteBindStatus QuoteBind 的生命周期状态。
type QuoteBindStatus string

const (
	// QuoteBindOpen 已颁发，等待结算。
	QuoteBindOpen QuoteBindStatus = "open"
	// QuoteBindSettled 结算完成，BPS 已按此 Quote 扣取。
	QuoteBindSettled QuoteBindStatus = "settled"
	// QuoteBindVoided 世代切换后主动作废，不得再用于结算。
	QuoteBindVoided QuoteBindStatus = "voided"
	// QuoteBindExpired TTL 到期自动失效。
	QuoteBindExpired QuoteBindStatus = "expired"
)

// QuoteBind 将一次 QuoteWithdraw 报价与档位世代绑定。
// 绑定协议：
// 1. QuoteWithdraw 时：读取当前 TierGeneration.Generation，颁发 QuoteBind（状态 open）。
// 2. 结算时：读取当前 TierGeneration.Generation，与 QuoteBind.Generation 比较。
// 不等 → ErrQuoteGenMismatch（档位已切换，旧 Quote 拒绝结算）。
// 3. TTL 到期（默认 5 分钟）→ 自动 expired，不得用于结算。
// 4. 档位切换时：将该用户所有 open QuoteBind 标记为 voided。
type QuoteBind struct {
	ID string `json:"quote_bind_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Gross string `json:"gross"`
	Fee   string `json:"fee"`
	Net string `json:"net"`
	BPS        int   `json:"bps"`
	Tier       Tier  `json:"tier"`
	Generation int64 `json:"generation"`
	LeaseID string `json:"lease_id,omitempty"`
	Status QuoteBindStatus `json:"status"`
	ExpiresAt time.Time  `json:"expires_at"`
	SettledAt *time.Time `json:"settled_at,omitempty"`
	VoidedAt  *time.Time `json:"voided_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

   func (q QuoteBind) Copy() QuoteBind { return q }

// IsUsable 返回 true 若 QuoteBind 当前可用于结算。

  func (q QuoteBind) IsUsable(now time.Time) bool {
	   return q.Status == QuoteBindOpen && now.Before(q.ExpiresAt)
  }

// ─────────────────────────────────────────────────────────────────────────────
// DriftCase
// ─────────────────────────────────────────────────────────────────────────────
// DriftCaseStatus DriftCase 的生命周期状态。
type DriftCaseStatus string

const (
	// DriftOpen 发现漂移，待证明与四眼收口。
	DriftOpen DriftCaseStatus = "open"
	// DriftProven 证明已附，等待两名操作员四眼确认。
	DriftProven DriftCaseStatus = "proven"
	// DriftClosed 四眼收口完成，漂移案关闭。
	DriftClosed DriftCaseStatus = "closed"
)

// DriftKind 漂移类型标识。

type DriftKind string

const (
	// DriftBPSStale 结算时用的 BPS 来自旧档，但账本占用已按新档计算。
	DriftBPSStale DriftKind = "bps_stale"
	// DriftFreezeStale 冻额按旧档 BPS 锁定，但在档位切换后未重算。
	DriftFreezeStale DriftKind = "freeze_stale"
	// DriftBPSReverse 反向漂移：账本 BPS 高于新档（抬档未生效）。
	DriftBPSReverse DriftKind = "bps_reverse"
	// DriftQuoteGenStale QuoteBind 世代与当前世代不符，但结算已尝试推进。
	DriftQuoteGenStale DriftKind = "quote_gen_stale"
)

// DriftCase 漂移案，记录一次检测到的档位漂移事故。
// 状态机：
// open ──→ proven ──→ closed (证明通过 + 四眼收口)
// open ──→ closed (仅在四眼直接否决且无需证明时，业务规则罕见)
// 存在 open 或 proven 状态的 DriftCase 时，同用户后续抬档结算操作被挡写，
// 直至四眼收口将案件关闭（DriftClosed）。

type DriftCase struct {
	ID string `json:"drift_case_id"`
	UserID       string    `json:"user_id"`
	Asset        string    `json:"asset"`
	Kind         DriftKind `json:"kind"`
	ObservedTier Tier      `json:"observed_tier"` // 结算/冻额实际所用档位
	ExpectedTier Tier      `json:"expected_tier"` // 当前世代对应的正确档位
	ObservedBPS  int       `json:"observed_bps"`
	ExpectedBPS int `json:"expected_bps"`
	Amount      string `json:"amount"` // 涉及金额（出金 gross）
	QuoteBindID string `json:"quote_bind_id,omitempty"`
	LeaseID     string `json:"lease_id,omitempty"`
	Generation int64 `json:"generation"` // 检测时快照的世代
	Status         DriftCaseStatus `json:"status"`
	ProofID        string          `json:"proof_id,omitempty"`
	FirstConfirmBy string          `json:"first_confirm_by,omitempty"`
	SecondConfirmBy string     `json:"second_confirm_by,omitempty"`
	FirstConfirmAt  *time.Time `json:"first_confirm_at,omitempty"`
	SecondConfirmAt *time.Time `json:"second_confirm_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	Note       string    `json:"note,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

  func (d DriftCase) Copy() DriftCase { return d }

// IsBlocking 返回 true 若此漂移案当前挡住了同用户的后续结算。

  func (d DriftCase) IsBlocking() bool {
	   return d.Status == DriftOpen || d.Status == DriftProven
   }

// ─────────────────────────────────────────────────────────────────────────────
// DriftProof
// ─────────────────────────────────────────────────────────────────────────────
// DriftProofStatus 漂移证明验证状态。

type DriftProofStatus string

const (
	DriftProofPending DriftProofStatus = "pending"
	DriftProofVerified DriftProofStatus = "verified"
	DriftProofFailed DriftProofStatus = "failed"
)

// DriftProof 漂移事故证明记录。
// 证明字段：
// Digest = SHA-256(drift_case_id ‖ user_id ‖ asset ‖ observed_bps ‖ expected_bps
// ‖ amount ‖ quote_bind_id ‖ lease_id ‖ generation)
// 验证条件：
// 1. Digest 与字段重计算一致（防篡改）。
// 2. ObservedBPS ≠ ExpectedBPS（漂移确实存在）。
// 3. DriftCase 处于 open 状态（不得重复打证明）。

type DriftProof struct {
	ID     string `json:"proof_id"`
	CaseID string `json:"drift_case_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	ObservedBPS int    `json:"observed_bps"`
	ExpectedBPS int    `json:"expected_bps"`
	Amount      string `json:"amount"`
	QuoteBindID string `json:"quote_bind_id,omitempty"`
	LeaseID     string `json:"lease_id,omitempty"`
	Generation int64 `json:"generation"`
	Digest     string           `json:"digest"`
	Status     DriftProofStatus `json:"status"`
	VerifiedAt *time.Time       `json:"verified_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

  func (p DriftProof) Copy() DriftProof { return p }

// ─────────────────────────────────────────────────────────────────────────────
// InflightFreeze — 在途冻额观察记录（只读挂钩）
// ─────────────────────────────────────────────────────────────────────────────
// InflightFreeze 表示一笔在途出金冻额的观察记录。
// 由外部出金模块写入；viptiergate 只读消费，不修改出金广播 FSM。
type InflightFreeze struct {
	ID string `json:"freeze_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	BPS int `json:"bps"`
	Tier       Tier      `json:"tier"`
	Generation int64     `json:"generation"`
	CreatedAt  time.Time `json:"created_at"`
	// ReleasedAt 非 nil 表示该冻额已被释放（出金成功/失败回滚）。
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

// IsActive 返回 true 若此冻额仍在途（未释放）。
  func (f InflightFreeze) IsActive() bool {
	   return f.ReleasedAt == nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// 请求/响应体
// ─────────────────────────────────────────────────────────────────────────────
// AcquireLeaseReq 申请档位切换租约的入参。

type AcquireLeaseReq struct {
	UserID string `json:"user_id"`
	FromTier Tier `json:"from_tier"`
	ToTier   Tier `json:"to_tier"`
	TriggerKind string `json:"trigger_kind"`
	TriggerRef string `json:"trigger_ref"`
	// ExpectedGen 调用方预期的当前世代号；用于 CAS ，防并发双抬档。
	ExpectedGen int64 `json:"expected_gen"`
}

// BindLeaseReq 将 open 租约推进至 bound 状态的入参。

type BindLeaseReq struct {
	LeaseID    string `json:"lease_id"`
	Generation int64  `json:"generation"` // 新世代号（Acquire 成功后递增的值）
}

// IssueQuoteReq 发起 QuoteBind 报价绑定的入参。
type IssueQuoteReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Gross  string `json:"gross"`
	Fee    string `json:"fee"`
	Net string `json:"net"`
	BPS  int  `json:"bps"`
	Tier Tier `json:"tier"`
	// TTL 报价有效期；0 使用默认值（5 分钟）。
	TTL int64 `json:"ttl_seconds,omitempty"`
}

// SettleQuoteReq 提交 QuoteBind 结算的入参。

type SettleQuoteReq struct {
	QuoteBindID string `json:"quote_bind_id"`
	UserID string `json:"user_id"`
	// CurrentGen 提交时读取的当前世代；必须与 QuoteBind.Generation 相等。
	CurrentGen int64 `json:"current_gen"`
}

// OpenDriftCaseReq 开立漂移案的入参。

type OpenDriftCaseReq struct {
	UserID       string    `json:"user_id"`
	Asset        string    `json:"asset"`
	Kind         DriftKind `json:"kind"`
	ObservedTier Tier      `json:"observed_tier"`
	ExpectedTier Tier      `json:"expected_tier"`
	ObservedBPS  int       `json:"observed_bps"`
	ExpectedBPS int    `json:"expected_bps"`
	Amount      string `json:"amount"`
	QuoteBindID string `json:"quote_bind_id,omitempty"`
	LeaseID     string `json:"lease_id,omitempty"`
	Generation  int64  `json:"generation"`
	Note string `json:"note,omitempty"`
}

// AttachProofReq 为漂移案附加证明的入参。

type AttachProofReq struct {
	CaseID string `json:"drift_case_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	ObservedBPS int    `json:"observed_bps"`
	ExpectedBPS int    `json:"expected_bps"`
	Amount      string `json:"amount"`
	QuoteBindID string `json:"quote_bind_id,omitempty"`
	LeaseID    string `json:"lease_id,omitempty"`
	Generation int64  `json:"generation"`
}

// ConfirmDriftReq 四眼收口确认漂移案的入参。
type ConfirmDriftReq struct {
	CaseID string `json:"drift_case_id"`
	Operator string `json:"operator"`
	Note string `json:"note,omitempty"`
}

// GenToken 世代令牌，随 QuoteBind 或外部调用携带，用于在途一致性验证。

type GenToken struct {
	UserID     string `json:"user_id"`
	Generation int64  `json:"generation"`
}
