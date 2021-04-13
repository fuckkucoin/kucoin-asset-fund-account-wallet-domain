package cvquotelease

import (
	"strings"
	"time"
)

// LeaseState 租约状态：issued → consumed | expired | revoked

type LeaseState string

const (
	LeaseIssued   LeaseState = "issued"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

const (
	LeaseEvConsume = "consume"
	LeaseEvExpire  = "expire"
	LeaseEvRevoke  = "revoke"
)

// DriftKind 成交封堵证明类别。

type DriftKind string

const (
	DriftMarkGen DriftKind = "mark_gen"
	DriftRate DriftKind = "rate"
	DriftBook DriftKind = "book"
	DriftFingerprint DriftKind = "fingerprint"
	DriftExpired DriftKind = "expired"
	DriftMissingLease DriftKind = "missing_lease"
	DriftDualExecute  DriftKind = "dual_execute"
	DriftReviveUsed DriftKind = "revive_used"
	DriftRevoked    DriftKind = "revoked"
	DriftOwner      DriftKind = "owner"
)

const (
	DefaultQuoteTTL    = 15 * time.Second
	DefaultExpiryBatch = 100
	DefaultRevokeBatch = 80
	KindLease          = "cv_quote_lease"
	KindMarkGen = "cv_mark_gen"
	KindProof     = "cv_quote_proof"
	KindQuoteSnap = "cv_quote_snap"
)

// CvQuoteLease Quote 签发的牌价指纹租约。
type CvQuoteLease struct {
	ID      string `json:"cv_quote_lease_id"`
	QuoteID string `json:"quote_id"`
	UserID  string `json:"user_id"`
	FromAsset string `json:"from_asset"`
	ToAsset   string `json:"to_asset"`
	Amount string `json:"amount"`
	Rate string `json:"rate"`
	Book string `json:"book"`
	MarkGen int64 `json:"mark_gen"`
	Fingerprint string     `json:"fingerprint"`
	BookPrint   string     `json:"book_print"`
	State       LeaseState `json:"state"`
	TTL Duration `json:"ttl"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ExpiredAt  *time.Time `json:"expired_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	Used    bool   `json:"used"`
	OrderID string `json:"order_id,omitempty"`
	ProofID string `json:"proof_id,omitempty"`
	RevokeReason string `json:"revoke_reason,omitempty"`
	ConsumeToken string `json:"consume_token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MarkGeneration 牌价源世代。MarkPut 必须抬升，使旧租约 Execute 失败。
type MarkGeneration struct {
	PairKey   string `json:"pair_key"`
	FromAsset string `json:"from_asset"`
	ToAsset   string `json:"to_asset"`
	Gen       int64  `json:"mark_gen"`
	Rate      string `json:"rate"`
	BookPrint string `json:"book_print"`
	Source    string `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy   string `json:"updated_by,omitempty"`
	RevokeCount int    `json:"revoke_count"`
}

// CvQuoteDriftProof 报价后成交封堵证明。
	Sealed  bool   `json:"sealed"`
	Detail  string `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// QuoteSnapshot Quote 侧只存 TTL/Used 的镜像，成交必须走租约复核。
type QuoteSnapshot struct {
	ID      string `json:"id"`
	LeaseID string `json:"cv_quote_lease_id"`
	UserID  string `json:"user_id"`
	FromAsset string `json:"from_asset"`
	ToAsset string `json:"to_asset"`
	Amount  string `json:"amount"`
	Rate string `json:"rate"`
	Book    string `json:"book"`
	MarkGen int64  `json:"mark_gen"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	OrderID string `json:"order_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Duration JSON 可序列化的 TTL。

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

   func NormalizeBook(book string) (string, error) {
	  book = strings.ToLower(strings.TrimSpace(book))
	   if book == "" {
		   book = "funding"
	   }
	   switch book {
	  case "funding", "trading", "both":
		   return book, nil
	   default:
		   return "", ErrBookInvalid
	   }
   }

  func NormalizeAsset(s string) string {
	   return strings.ToUpper(strings.TrimSpace(s))
  }

  func PairKey(from, to string) string {
	  return NormalizeAsset(from) + "|" + NormalizeAsset(to)
  }

  func IsTerminal(s LeaseState) bool {
	   return s == LeaseConsumed || s == LeaseExpired || s == LeaseRevoked
  }

   func IsIssued(s LeaseState) bool {
	  return s == LeaseIssued
  }

// QuoteWithLeaseReq / Resp
type QuoteWithLeaseReq struct {
	UserID string `json:"user_id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount string `json:"amount"`
	Book string `json:"book"`
	TTL string `json:"ttl,omitempty"`
}

type QuoteWithLeaseResp struct {
	QuoteID string `json:"quote_id"`
	LeaseID string `json:"cv_quote_lease_id"`
	UserID string `json:"user_id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount string `json:"amount"`
	Rate string `json:"rate"`
	Book string `json:"book"`
	MarkGen int64 `json:"mark_gen"`
	Fingerprint string    `json:"fingerprint"`
	ExpiresAt   time.Time `json:"expires_at"`
	Used bool `json:"used"`
}

type ExecuteWithLeaseReq struct {
	UserID  string `json:"user_id"`
	QuoteID string `json:"quote_id"`
	LeaseID string `json:"cv_quote_lease_id"`
}

type ExecuteWithLeaseResp struct {
	OrderID string `json:"order_id,omitempty"`
	QuoteID string `json:"quote_id"`
	LeaseID string     `json:"cv_quote_lease_id"`
	State   LeaseState `json:"state"`
	Filled  bool       `json:"filled"`
	Reused  bool       `json:"reused"`
	Rate    string     `json:"rate,omitempty"`
	Book string `json:"book,omitempty"`
	DriftProof *CvQuoteDriftProof `json:"drift_proof,omitempty"`
	Used bool `json:"used"`
}

type MarkPutReq struct {
	From string `json:"from"`
	To     string `json:"to"`
	Rate   string `json:"rate"`
	Source string `json:"source,omitempty"`
	By string `json:"updated_by,omitempty"`
}

type MarkPutResp struct {
	PairKey string `json:"pair_key"`
	MarkGen int64 `json:"mark_gen"`
	Rate    string `json:"rate"`
	Revoked int    `json:"revoked_leases"`
	BookPrint string `json:"book_print"`
}

type PlaceQuotedReq struct {
	UserID string `json:"user_id"`
	From   string `json:"from"`
	To string `json:"to"`
	Amount string `json:"amount"`
	Rate string `json:"rate"`
	Book string `json:"book"`
	QuoteID string `json:"quote_id"`
	LeaseID   string `json:"cv_quote_lease_id"`
	AllowBare bool   `json:"allow_bare"`
}

type ProofExportReq struct {
	LeaseID string `json:"lease_id,omitempty"`
	QuoteID string `json:"quote_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Limit int `json:"limit,omitempty"`
}

type ProofExportResp struct {
	Proofs []CvQuoteDriftProof `json:"proofs"`
	Count  int                 `json:"count"`
}

type HealthResp struct {
	IssuedLeases   int `json:"issued_leases"`
	ConsumedLeases int `json:"consumed_leases"`
	ExpiredLeases  int `json:"expired_leases"`
	RevokedLeases int `json:"revoked_leases"`
	TotalProofs   int `json:"total_drift_proofs"`
	MarkPairs int `json:"mark_pairs"`
}

type FillResult struct {
	OrderID string
	Rate string
	Book string
}
