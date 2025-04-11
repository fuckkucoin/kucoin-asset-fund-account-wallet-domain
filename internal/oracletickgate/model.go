package oracletickgate

import "time"

// LeaseState 估值租约状态。

type LeaseState string

const (
	LeaseIssued   LeaseState = "issued"
	LeaseBound    LeaseState = "bound"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

// Terminal 终态不得再绑资金写。

   func (s LeaseState) Terminal() bool {
	   return s == LeaseConsumed || s == LeaseExpired || s == LeaseRevoked
  }

// OpenForBind 是否允许进入资金写绑定。

   func (s LeaseState) OpenForBind() bool {
	   return s == LeaseIssued
   }

// OpenForConsume 绑定后是否允许落账消费。

  func (s LeaseState) OpenForConsume() bool {
	   return s == LeaseBound
   }

// ValuationKind 估值入口。只覆盖 Convert / MarkToMarket / MarginLTV。
type ValuationKind string

const (
	ValConvert ConvertKind = "convert"
	ValMTM     ConvertKind = "mark_to_market"
	ValLTV     ConvertKind = "margin_ltv"
)

// ConvertKind 与 ValuationKind 同形，便于 JSON。
type ConvertKind = ValuationKind

// LeaseMode 签发模式。Shadow/Probe 不得冒充 Enforce 写租约。

type LeaseMode string

const (
	ModeEnforce LeaseMode = "enforce"
	ModeShadow LeaseMode = "shadow"
	ModeProbe  LeaseMode = "probe"
)

// Writable 仅 Enforce 可进入资金写成功路径。

   func (m LeaseMode) Writable() bool {
	  return m == ModeEnforce
  }

// FundWriteKind 依赖估值的资金写种类。
type FundWriteKind string

const (
	WriteCollateralMTM FundWriteKind = "collateral_mtm_post"
	WriteLTVLiquidate FundWriteKind = "ltv_liquidation_write"
	WriteConvertPost FundWriteKind = "convert_post"
)

// TickLease 估值世代钉扎。对外只读标识：tick_lease_id。

type TickLease struct {
	LeaseID string `json:"tick_lease_id"`
	Pair string `json:"pair"`
	Asset string `json:"asset"`
	Quote  string    `json:"quote"`
	Price  string    `json:"price"`
	TickAt time.Time `json:"tick_at"`
	Fingerprint string `json:"tick_fingerprint"`
	Generation int64 `json:"generation"`
	ValuationKind ValuationKind `json:"valuation_kind"`
	Mode LeaseMode `json:"mode"`
	QuoteCcy string `json:"quote_ccy"`
	Amount string `json:"amount,omitempty"`
	BorrowAsset string `json:"borrow_asset,omitempty"`
	BorrowAmt string `json:"borrow_amt,omitempty"`
	ColAsset  string `json:"col_asset,omitempty"`
	ColAmt string `json:"col_amt,omitempty"`
	MTMTotal string     `json:"mtm_total,omitempty"`
	LTVBPS   int        `json:"ltv_bps,omitempty"`
	State    LeaseState `json:"state"`
	TTLSeconds int64 `json:"ttl_seconds"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	BoundAt    *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	WriteKind FundWriteKind `json:"write_kind,omitempty"`
	WriteRef string `json:"write_ref,omitempty"`
	Actor    string `json:"actor,omitempty"`
	Note     string `json:"note,omitempty"`
	IdemKey string `json:"idem_key,omitempty"`
}

// Copy 值拷贝。
   func (l TickLease) Copy() TickLease { return l }

// Alive 未过期且非终态。
  func (l TickLease) Alive(now time.Time) bool {
	   if l.State.Terminal() {
		  return false
	  }
	  if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
		  return false
	  }
	   return true
   }

// TickGeneration pair 世代视图。
type TickGeneration struct {
	Pair string `json:"pair"`
	Generation int64 `json:"generation"`
	Price string `json:"price"`
	TickAt    time.Time `json:"tick_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProofKind 证明类型。

type ProofKind string

const (
	ProofTickFlip    ProofKind = "tick_price_flip"
	ProofTickStale   ProofKind = "tick_at_stale"
	ProofTickMissing ProofKind = "tick_missing"
	ProofPriceDrift  ProofKind = "tick_price_drift"
	ProofGeneration  ProofKind = "tick_generation_mismatch"
	ProofShadowImpersonate ProofKind = "shadow_impersonate_enforce"
	ProofNoLease ProofKind = "fund_write_without_lease"
	ProofLeaseExpired      ProofKind = "lease_expired_revoked"
	ProofMissingPriceIssue ProofKind = "missing_price_no_enforce_lease"
	ProofModeRefuse        ProofKind = "probe_or_shadow_refused"
	ProofWriteKindMismatch ProofKind = "write_kind_mismatch"
	ProofIdemReplay ProofKind = "idempotent_replay"
)

// TickDriftProof 可导出拒写证明。
type TickDriftProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"tick_lease_id"`
	Pair    string `json:"pair"`
	Kind        ProofKind `json:"kind"`
	FailClosed  bool      `json:"fail_closed"`
	IssuedPrice string    `json:"issued_price,omitempty"`
	LivePrice string `json:"live_price,omitempty"`
	IssuedGen int64  `json:"issued_generation"`
	LiveGen   int64  `json:"live_generation"`
	IssuedFP  string `json:"issued_fingerprint,omitempty"`
	LiveFP string `json:"live_fingerprint,omitempty"`
	WriteKind string `json:"write_kind,omitempty"`
	Summary string `json:"summary"`
	OpenedAt   time.Time  `json:"opened_at"`
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	Detail        string `json:"detail,omitempty"`
	FundWriteHeld bool   `json:"fund_write_held"`
}

// AuditEvent 估值租约审计。
type AuditEvent struct {
	AuditID string `json:"audit_id"`
	LeaseID string `json:"tick_lease_id,omitempty"`
	Pair    string `json:"pair"`
	Action string `json:"action"`
	Actor string `json:"actor"`
	FromState string `json:"from_state,omitempty"`
	ToState string `json:"to_state,omitempty"`
	Detail  string `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IssueTickLeaseRequest 签发请求。
type IssueTickLeaseRequest struct {
	Asset string `json:"asset"`
	Quote         string        `json:"quote"`
	Amount        string        `json:"amount,omitempty"`
	ValuationKind ValuationKind `json:"valuation_kind"`
	Mode        LeaseMode `json:"mode"`
	BorrowAsset string    `json:"borrow_asset,omitempty"`
	BorrowAmt   string    `json:"borrow_amt,omitempty"`
	ColAsset    string    `json:"col_asset,omitempty"`
	ColAmt      string    `json:"col_amt,omitempty"`
	Amounts map[string]string `json:"amounts,omitempty"`
	TTLSeconds int64 `json:"ttl_seconds,omitempty"`
	Actor   string `json:"actor"`
	IdemKey string `json:"idem_key,omitempty"`
}

// AssertFundWriteRequest 资金写前复验。必须携带未过期 tick_lease。

type AssertFundWriteRequest struct {
	LeaseID string `json:"tick_lease_id"`
	WriteKind FundWriteKind `json:"write_kind"`
	WriteRef string `json:"write_ref,omitempty"`
	Actor    string `json:"actor"`
	IdemKey  string `json:"idem_key,omitempty"`
}

// RevokeLeaseRequest 运维吊销。
type RevokeLeaseRequest struct {
	LeaseID string `json:"tick_lease_id"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// WorkerTickResult 巡检结果。

type WorkerTickResult struct {
	ExpiredRevoked int `json:"expired_revoked"`
	FlipRevoked    int `json:"flip_revoked"`
	StaleRevoked int `json:"stale_revoked"`
	ProofsOpened int `json:"proofs_opened"`
	GensScanned int `json:"generations_scanned"`
}

// LeaseView 只读视图。
type LeaseView struct {
	Lease  *TickLease       `json:"lease,omitempty"`
	Gen    *TickGeneration  `json:"generation,omitempty"`
	Proofs []TickDriftProof `json:"proofs,omitempty"`
	Audits []AuditEvent `json:"audits,omitempty"`
}

// FundWriteRecord 成功资金写记录（本包不入账本，只记绑定结果）。

type FundWriteRecord struct {
	WriteID   string        `json:"write_id"`
	LeaseID   string        `json:"tick_lease_id"`
	WriteKind FundWriteKind `json:"write_kind"`
	WriteRef  string        `json:"write_ref"`
	Pair string `json:"pair"`
	Generation int64 `json:"generation"`
	Posted     bool  `json:"posted"`
	Actor    string    `json:"actor"`
	PostedAt time.Time `json:"posted_at"`
}

// DefaultLeaseTTL 估值租约默认存活。
const DefaultLeaseTTL = int64(30)

// DefaultDriftBPS 价格漂移阈值（基点）。超过即吊销旧租约。
const DefaultDriftBPS = 50
