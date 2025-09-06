package fxmarkleasegate

import (
	"strings"
	"time"
)

// LeaseState issued → bound → consumed | expired | revoked。
type LeaseState string

const (
	LeaseIssued   LeaseState = "issued"
	LeaseBound    LeaseState = "bound"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

const (
	LeaseEvIssue = "issue"
	LeaseEvBind  = "bind"
	LeaseEvConsume = "consume"
	LeaseEvExpire = "expire"
	LeaseEvRevoke = "revoke"
)

// DriftKind 落账封堵证明类别。
type DriftKind string

const (
	DriftPutChange      DriftKind = "put_change"
	DriftLiveOverThresh DriftKind = "live_over_threshold"
	DriftFingerprint    DriftKind = "fingerprint"
	DriftGeneration     DriftKind = "generation"
	DriftExpired DriftKind = "expired"
	DriftMissingLease DriftKind = "missing_lease"
	DriftMissingRate DriftKind = "missing_rate"
	DriftRevoked     DriftKind = "revoked"
	DriftBinding     DriftKind = "binding"
	DriftAlreadyConsumed DriftKind = "already_consumed"
	DriftNotBound   DriftKind = "not_bound"
	DriftSourceFlip DriftKind = "source_flip"
	DriftHTTPLive   DriftKind = "http_live_drift"
)

const (
	DefaultLeaseTTL    = 20 * time.Second
	DefaultExpiryBatch = 100
	DefaultPatrolBatch  = 80
	DefaultThresholdBPS = int64(10)
	KindLease           = "fx_mark_lease"
	KindProof      = "fx_mark_drift_proof"
	KindAudit      = "fx_mark_audit"
	KindGeneration = "fx_mark_generation"
	TableLeases    = "fx_mark_leases"
	TableProofs    = "fx_mark_drift_proofs"
	TableAudit     = "fx_mark_audit"
	TableGeneration = "fx_mark_generations"
)

const (
	PurposeConvert  = "convert"
	PurposeFundPage = "fund_page"
	PurposeValuation = "valuation"
	PurposeTravel    = "travel_nominal"
)

const (
	SourceKV = "kv"
	SourceHTTP = "http"
	SourceLive = "live"
	SourceStatic = "static"
	SourceStable = "stable"
)

// MarkLease 资金落账读取 Rate/USD 时签发的牌价租约。
type MarkLease struct {
	ID      string `json:"mark_lease_id"`
	From    string `json:"from"`
	To      string `json:"to"`
	FromUSD string `json:"from_usd"`
	ToUSD       string `json:"to_usd"`
	Rate        string `json:"rate"`
	Source      string `json:"source"`
	SourcePrint string `json:"source_fingerprint"`
	PairPrint string `json:"pair_fingerprint"`
	Generation int64 `json:"generation"`
	FromGen    int64 `json:"from_generation"`
	ToGen      int64 `json:"to_generation"`
	ThresholdBPS int64      `json:"threshold_bps"`
	Purpose      string     `json:"purpose"`
	State        LeaseState `json:"state"`
	TTL Duration `json:"ttl"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	BoundAt    *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ExpiredAt  *time.Time `json:"expired_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	ProofID      string     `json:"proof_id,omitempty"`
	Idem         string     `json:"idem,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
	PostRef   string    `json:"post_ref,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MarkGeneration 资产牌价世代；Put 同资产改价递增。
type MarkGeneration struct {
	Asset string `json:"asset"`
	Seq   int64  `json:"seq"`
	LastUSD   string    `json:"last_usd"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FxMarkDriftProof 牌价漂移封堵证明。
type FxMarkDriftProof struct {
	ID string `json:"id"`
	LeaseID string    `json:"mark_lease_id"`
	Kind    DriftKind `json:"kind"`
	From string `json:"from"`
	To string `json:"to"`
	LeaseRate  string `json:"lease_rate"`
	LiveRate   string `json:"live_rate"`
	LeasePrint string `json:"lease_source_fingerprint"`
	LivePrint string `json:"live_source_fingerprint"`
	LeaseGen  int64  `json:"lease_generation"`
	LiveGen   int64  `json:"live_generation"`
	DriftBPS int64 `json:"drift_bps"`
	ThresholdBPS int64 `json:"threshold_bps"`
	Blocked      bool  `json:"blocked"`
	Sealed       bool  `json:"sealed"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// MarkLeaseAudit 租约审计流水。

type MarkLeaseAudit struct {
	ID string `json:"id"`
	LeaseID string `json:"mark_lease_id"`
	Event     string     `json:"event"`
	FromState LeaseState `json:"from_state"`
	ToState   LeaseState `json:"to_state"`
	Actor     string     `json:"actor"`
	Detail    string     `json:"detail"`
	At time.Time `json:"at"`
}

// Duration JSON 可序列化 TTL。
type Duration struct {
	time.Duration
}

   func (d Duration) MarshalJSON() ([]byte, error) {
	  return []byte(`"` + d.Duration.String() + `"`), nil
   }

   func (d *Duration) UnmarshalJSON(b []byte) error {
	  s := string(b)
	  if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		  s = s[1 : len(s)-1]
	  }
	  dur, err := time.ParseDuration(s)
	  if err != nil {
		  return err
	  }
	   d.Duration = dur
	  return nil
   }

   func NormalizePair(from, to string) (string, string, error) {
	   from = strings.ToUpper(strings.TrimSpace(from))
	   to = strings.ToUpper(strings.TrimSpace(to))
	   if from == "" || to == "" {
		  return "", "", ErrPairRequired
	   }
	  if from == to {
		  return "", "", ErrPairInvalid
	  }
	   return from, to, nil
  }

   func NormalizePurpose(p string) (string, error) {
	   switch strings.ToLower(strings.TrimSpace(p)) {
	   case "", PurposeConvert:
		   return PurposeConvert, nil
	   case PurposeFundPage, "fund-page", "assets":
		  return PurposeFundPage, nil
	   case PurposeValuation, "value":
		   return PurposeValuation, nil
	   case PurposeTravel, "travel":
		  return PurposeTravel, nil
	  default:
		  return "", ErrPurposeInvalid
	  }
  }

  func PairKey(from, to string) string {
	  return strings.ToUpper(strings.TrimSpace(from)) + "|" + strings.ToUpper(strings.TrimSpace(to))
   }

   func IsTerminal(s LeaseState) bool {
	  return s == LeaseConsumed || s == LeaseExpired || s == LeaseRevoked
  }

   func IsLive(s LeaseState) bool {
	   return s == LeaseIssued || s == LeaseBound
  }

// IssueMarkLeaseReq 签发牌价租约。

type IssueMarkLeaseReq struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Purpose      string `json:"purpose,omitempty"`
	TTL          string `json:"ttl,omitempty"`
	ThresholdBPS int64  `json:"threshold_bps,omitempty"`
	Idem       string `json:"idem,omitempty"`
	Actor      string `json:"actor,omitempty"`
	PreferLive bool   `json:"prefer_live"`
}

type IssueMarkLeaseResp struct {
	LeaseID string `json:"mark_lease_id"`
	From    string `json:"from"`
	To string `json:"to"`
	Rate string `json:"rate"`
	FromUSD string `json:"from_usd"`
	ToUSD   string `json:"to_usd"`
	Source       string `json:"source"`
	SourcePrint  string `json:"source_fingerprint"`
	Generation   int64  `json:"generation"`
	ThresholdBPS int64  `json:"threshold_bps"`
	State LeaseState `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	Purpose   string    `json:"purpose"`
}

type BindAssertReq struct {
	LeaseID string `json:"mark_lease_id"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Purpose string `json:"purpose,omitempty"`
	PostRef string `json:"post_ref,omitempty"`
	Actor string `json:"actor,omitempty"`
}

type BindAssertResp struct {
	LeaseID string     `json:"mark_lease_id"`
	State   LeaseState `json:"state"`
	Bound bool `json:"bound"`
	Rate      string    `json:"rate"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AssertBeforePostReq 换算/资金页估值落账前复核。

type AssertBeforePostReq struct {
	LeaseID string `json:"mark_lease_id"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Purpose string `json:"purpose,omitempty"`
	PostRef string `json:"post_ref,omitempty"`
	Actor string `json:"actor,omitempty"`
	Consume bool `json:"consume"`
}

type AssertBeforePostResp struct {
	LeaseID string     `json:"mark_lease_id"`
	State   LeaseState `json:"state"`
	Posted     bool              `json:"posted"`
	Rate       string            `json:"rate"`
	Source     string            `json:"source"`
	DriftProof *FxMarkDriftProof `json:"drift_proof,omitempty"`
	Reused     bool              `json:"reused"`
}

type ProofExportReq struct {
	LeaseID string `json:"lease_id,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type ProofExportResp struct {
	Proofs []FxMarkDriftProof `json:"proofs"`
	Count  int                `json:"count"`
}

type OpsRevokeReq struct {
	LeaseID string `json:"mark_lease_id"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

type OpsRevokeResp struct {
	LeaseID string `json:"mark_lease_id"`
	State LeaseState `json:"state"`
	Revoked bool `json:"revoked"`
	Reason string `json:"reason"`
}

type HealthResp struct {
	Issued int `json:"issued_leases"`
	Bound int `json:"bound_leases"`
	Consumed int `json:"consumed_leases"`
	Expired int `json:"expired_leases"`
	Revoked int `json:"revoked_leases"`
	Proofs  int `json:"total_drift_proofs"`
	Audits  int `json:"audit_rows"`
	Gens    int `json:"generation_rows"`
}

type LiveSnap struct {
	From string
	To   string
	FromUSD string
	ToUSD string
	Rate string
	Source string
	Generation int64
	FromGen    int64
	ToGen      int64
}
