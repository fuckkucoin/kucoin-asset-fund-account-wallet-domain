package dustquotegate

import "time"

// LeaseState 尘兑报价租约状态。
type LeaseState string

const (
	LeaseQuoted LeaseState = "quoted"
	LeaseBound LeaseState = "bound"
	LeaseSettled LeaseState = "settled"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

// Terminal 终态不可逆。

   func (s LeaseState) Terminal() bool {
	   return s == LeaseSettled || s == LeaseExpired || s == LeaseRevoked
  }

// CanBind quoted 才可绑账。

  func (s LeaseState) CanBind() bool { return s == LeaseQuoted }

// CanSettle bound 才可 Conserved 落账。

   func (s LeaseState) CanSettle() bool { return s == LeaseBound }

// ProofKind 漂移/拒兑证明类型。

type ProofKind string

const (
	ProofRateDrift ProofKind = "rate_drift"
	ProofPolicyDrift ProofKind = "policy_drift"
	ProofBookDrift ProofKind = "book_drift"
	ProofCapDrift ProofKind = "cap_drift"
	ProofExpired  ProofKind = "expired_lease"
	ProofRevoked      ProofKind = "revoked_lease"
	ProofConserveFail ProofKind = "conserve_fail"
	ProofScanHalt     ProofKind = "scan_halt"
	ProofOpsRevoke    ProofKind = "ops_revoke"
	ProofNoLease      ProofKind = "no_lease"
	ProofBindOK ProofKind = "bind_ok"
	ProofSettleOK   ProofKind = "settle_ok"
	ProofDupBind    ProofKind = "dup_bind"
	ProofTTLExpired ProofKind = "ttl_expired"
)

// FlipKind 牌价/策略翻转种类。

type FlipKind string

const (
	FlipRate      FlipKind = "put_rate"
	FlipPolicyGen FlipKind = "put_policy_gen"
	FlipCapBPS    FlipKind = "put_cap_bps"
)

// PolicyRateFingerprint 绑定落账用的策略+牌价指纹。
// 不改写 floor/cap 公式，只钉扎当时快照。

type PolicyRateFingerprint struct {
	Asset       string `json:"asset"`
	Rate        string `json:"rate"`
	PolicyGen   int64  `json:"policy_gen"`
	CapBPS      int64  `json:"cap_bps"`
	FloorPinned string `json:"floor_pinned"`
	MinPinned   string `json:"min_convert_pinned"`
	BookFP      string `json:"book_fp"`
	BookAvail   string `json:"book_available"`
	Digest      string `json:"digest"`
}

// Copy 值拷贝。
  func (f PolicyRateFingerprint) Copy() PolicyRateFingerprint { return f }

// DustQuoteLease Convert 预检签发的尘兑牌价租约。
type DustQuoteLease struct {
	LeaseID string `json:"dust_quote_lease_id"`
	UserID  string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	USDTWant string `json:"usdt_want"`
	Rate      string `json:"rate"`
	PolicyGen int64  `json:"policy_gen"`
	CapBPS    int64  `json:"cap_bps"`
	BookFP string `json:"book_fp"`
	Digest string `json:"digest"`
	State  LeaseState `json:"state"`
	TTLSec int64      `json:"ttl_sec"`
	Idem      string `json:"idempotency_key"`
	Actor     string `json:"actor"`
	ScanBatch string `json:"scan_batch,omitempty"`
	ScanIndex int       `json:"scan_index,omitempty"`
	QuotedAt  time.Time `json:"quoted_at"`
	BoundAt   time.Time `json:"bound_at,omitempty"`
	SettledAt time.Time `json:"settled_at,omitempty"`
	ExpiredAt time.Time `json:"expired_at,omitempty"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`
	RevokeWhy string    `json:"revoke_why,omitempty"`
	FlipEpoch int64     `json:"flip_epoch"`
	ConvertID   string                `json:"convert_id,omitempty"`
	Fingerprint PolicyRateFingerprint `json:"fingerprint"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 租约值拷贝。

  func (l DustQuoteLease) Copy() DustQuoteLease {
	   out := l
	   out.Fingerprint = l.Fingerprint.Copy()
	   return out
  }

// DustQuoteDriftProof 拒兑/漂移不可变证明。
type DustQuoteDriftProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"dust_quote_lease_id"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Kind ProofKind `json:"kind"`
	QuotedRate string `json:"quoted_rate"`
	LiveRate   string `json:"live_rate"`
	QuotedGen  int64  `json:"quoted_policy_gen"`
	LiveGen    int64  `json:"live_policy_gen"`
	QuotedBook string `json:"quoted_book_fp"`
	LiveBook  string `json:"live_book_fp"`
	QuotedCap int64  `json:"quoted_cap_bps"`
	LiveCap   int64  `json:"live_cap_bps"`
	Detail    string `json:"detail"`
	Actor     string `json:"actor"`
	ScanBatch string `json:"scan_batch,omitempty"`
	At        time.Time `json:"at"`
	CreatedAt time.Time `json:"created_at"`
}

// Copy 证明值拷贝。

   func (p DustQuoteDriftProof) Copy() DustQuoteDriftProof { return p }

// DustQuoteAudit 租约审计流水。
type DustQuoteAudit struct {
	AuditID string `json:"audit_id"`
	LeaseID string `json:"dust_quote_lease_id"`
	Action  string `json:"action"`
	From string `json:"from_state"`
	To string `json:"to_state"`
	Actor  string    `json:"actor"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

// Copy 审计值拷贝。
   func (a DustQuoteAudit) Copy() DustQuoteAudit { return a }

// Scan ConvertScan 中断巡检游标。

type ScanCursor struct {
	BatchID string `json:"batch_id"`
	UserID string `json:"user_id"`
	NextIndex int  `json:"next_index"`
	Halted    bool `json:"halted"`
	HaltReason string `json:"halt_reason"`
	SettledN  int `json:"settled_n"`
	RejectedN int `json:"rejected_n"`
	FlipEpoch int64  `json:"flip_epoch"`
	Actor     string `json:"actor"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 游标值拷贝。

  func (c ScanCursor) Copy() ScanCursor { return c }

// FlipEvent 牌价/策略翻转事件（吊销泵输入）。
type FlipEvent struct {
	EventID string   `json:"event_id"`
	Asset   string   `json:"asset"`
	Kind    FlipKind `json:"kind"`
	OldRate string `json:"old_rate"`
	NewRate string `json:"new_rate"`
	OldGen  int64  `json:"old_gen"`
	NewGen int64     `json:"new_gen"`
	Epoch  int64     `json:"epoch"`
	At     time.Time `json:"at"`
}

// Copy 翻转事件值拷贝。
   func (e FlipEvent) Copy() FlipEvent { return e }

// QuoteReq 预检签发请求。

type QuoteReq struct {
	UserID string `json:"user_id"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	Actor          string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	TTLSec int64 `json:"ttl_sec"`
	ScanBatch string `json:"scan_batch,omitempty"`
	ScanIndex int    `json:"scan_index,omitempty"`
}

// BindReq 落账绑定请求。

type BindReq struct {
	LeaseID string `json:"dust_quote_lease_id"`
	UserID  string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Actor string `json:"actor"`
}

// SettleReq Conserved 后结清租约。
type SettleReq struct {
	LeaseID   string `json:"dust_quote_lease_id"`
	ConvertID string `json:"convert_id"`
	USDT string `json:"usdt"`
	Actor string `json:"actor"`
}

// RevokeReq 运维吊销。
type RevokeReq struct {
	LeaseID string `json:"dust_quote_lease_id"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// LeaseConfig 租约默认参数。
type LeaseConfig struct {
	DefaultTTL    time.Duration
	MaxTTL        time.Duration
	DefaultCapBPS int64
}

// Fill 填默认值。
   func (c *LeaseConfig) Fill() {
	  if c.DefaultTTL <= 0 {
		  c.DefaultTTL = 2 * time.Minute
	   }
	   if c.MaxTTL <= 0 {
		   c.MaxTTL = 15 * time.Minute
	  }
	   if c.DefaultCapBPS <= 0 {
		  c.DefaultCapBPS = 50
	   }
   }

// WorkerConfig 过期扫描、翻转吊销、Scan 巡检间隔。
type WorkerConfig struct {
	ExpiryScanInterval time.Duration
	FlipPumpInterval  time.Duration
	ScanWatchInterval time.Duration
	BatchSize int
	WorkerID  string
}

// Fill 填 Worker 默认值。

   func (c *WorkerConfig) Fill() {
	   if c.ExpiryScanInterval <= 0 {
		  c.ExpiryScanInterval = 15 * time.Second
	  }
	   if c.FlipPumpInterval <= 0 {
		   c.FlipPumpInterval = 5 * time.Second
	  }
	  if c.ScanWatchInterval <= 0 {
		   c.ScanWatchInterval = 10 * time.Second
	  }
	  if c.BatchSize <= 0 {
		  c.BatchSize = 200
	  }
	  if c.WorkerID == "" {
		  c.WorkerID = "dust-quote-gate.worker"
	   }
   }
