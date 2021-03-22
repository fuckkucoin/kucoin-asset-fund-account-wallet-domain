package booksnapgate

import (
	"time"
)

// ──────────────────────────────────────────
// BookSnapLease 账本快照租约
// ──────────────────────────────────────────

type LeaseState string

const (
	LeasePinned LeaseState = "pinned"
	LeaseAssembling LeaseState = "assembling"
	LeaseSealed LeaseState = "sealed"
	LeaseDrifted LeaseState = "drifted"
	LeaseExpired LeaseState = "expired"
	LeaseAborted LeaseState = "aborted"
)

const (
	LeaseEvBeginAssemble = "begin_assemble"
	LeaseEvSeal   = "seal"
	LeaseEvDrift  = "drift"
	LeaseEvExpire = "expire"
	LeaseEvAbort = "abort"
	LeaseEvRepin = "repin"
)

type BookSnapLease struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset      string `json:"asset"`
	SnapWindow string `json:"snap_window"`
	State LeaseState `json:"state"`
	SnapGen int64 `json:"snap_gen"`
	BookSnaps []BookSnap `json:"book_snaps"`
	PinTTL    Duration   `json:"pin_ttl"`
	PinnedAt  time.Time  `json:"pinned_at"`
	ExpiresAt time.Time `json:"expires_at"`
	AssembleStart *time.Time `json:"assemble_start,omitempty"`
	SealedAt      *time.Time `json:"sealed_at,omitempty"`
	DriftedAt     *time.Time `json:"drifted_at,omitempty"`
	AbortedAt *time.Time `json:"aborted_at,omitempty"`
	RepinCount int `json:"repin_count"`
	MaxRepin   int `json:"max_repin"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	SealedResult *MultiConserve `json:"sealed_result,omitempty"`
	DriftProofID string         `json:"drift_proof_id,omitempty"`
}

type BookSnap struct {
	Book   string `json:"book"`
	SnapID string `json:"snap_id"`
	ObserveGen int64 `json:"observe_gen"`
	Waterline string `json:"waterline"`
	Checksum string `json:"checksum"`
}

// Duration 用于 JSON 序列化 time.Duration。
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

// ──────────────────────────────────────────
// BookSnapDriftProof 漂移证明
// ──────────────────────────────────────────

type DriftKind string

const (
	DriftKindWaterline DriftKind = "waterline_shift"
	DriftKindGenMismatch DriftKind = "gen_mismatch"
	DriftKindCreditHop    DriftKind = "cross_book_credit_hop"
	DriftKindConserveFail DriftKind = "conserve_fail"
	DriftKindExpiry DriftKind = "lease_expiry"
	DriftKindAbort  DriftKind = "manual_abort"
)

type BookSnapDriftProof struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID string `json:"user_id"`
	Asset   string    `json:"asset"`
	SnapGen int64     `json:"snap_gen"`
	Kind    DriftKind `json:"kind"`
	Book    string    `json:"book,omitempty"`
	OldWaterline string `json:"old_waterline,omitempty"`
	NewWaterline string `json:"new_waterline,omitempty"`
	OldGen       int64  `json:"old_gen,omitempty"`
	NewGen int64 `json:"new_gen,omitempty"`
	DebitBefore  string    `json:"debit_before,omitempty"`
	CreditBefore string    `json:"credit_before,omitempty"`
	DebitAfter   string    `json:"debit_after,omitempty"`
	CreditAfter  string    `json:"credit_after,omitempty"`
	Delta        string    `json:"delta,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ──────────────────────────────────────────
// MultiConserve 多科目守恒结果
// ──────────────────────────────────────────

type BookTotals struct {
	Book string `json:"book"`
	Debit  string `json:"debit"`
	Credit string `json:"credit"`
	Count int `json:"count"`
}

type MultiConserve struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	SnapGen int64        `json:"snap_gen"`
	Books   []BookTotals `json:"books"`
	TotalDebit string `json:"total_debit"`
	TotalCredit string `json:"total_credit"`
	Conserved   bool   `json:"conserved"`
	Delta       string `json:"delta,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	SnapWindow string `json:"snap_window"`
	LeaseID    string `json:"lease_id"`
}

// ──────────────────────────────────────────
// SnapWindow 快照窗口标识
// ──────────────────────────────────────────

type SnapWindow struct {
	UserID string
	Asset string
	From  time.Time
	To    time.Time
}

   func (w SnapWindow) Key() string {
	   return w.UserID + "|" + w.Asset + "|" + w.From.UTC().Format(time.RFC3339) + "|" + w.To.UTC().Format(time.RFC3339)
   }

  func (w SnapWindow) Valid() bool {
	  return w.UserID != "" && w.Asset != "" && !w.From.IsZero() && !w.To.IsZero() && w.To.After(w.From)
   }

// ──────────────────────────────────────────
// WaterlineObservation 科目水线观测
// ──────────────────────────────────────────

type WaterlineObservation struct {
	Book string `json:"book"`
	Waterline  string `json:"waterline"`
	ObserveGen int64  `json:"observe_gen"`
	Checksum   string `json:"checksum"`
}

// ──────────────────────────────────────────
// Request / Response
// ──────────────────────────────────────────

type PinSnapReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	TTL    string `json:"ttl,omitempty"`
	Window string `json:"window,omitempty"`
}

type PinSnapResp struct {
	LeaseID string `json:"lease_id"`
	SnapGen int64      `json:"snap_gen"`
	State   LeaseState `json:"state"`
	BookSnaps []BookSnap `json:"book_snaps"`
	ExpiresAt time.Time  `json:"expires_at"`
	Reused    bool       `json:"reused"`
}

type AssembleReq struct {
	LeaseID string `json:"lease_id"`
}

type AssembleResp struct {
	LeaseID string     `json:"lease_id"`
	State   LeaseState `json:"state"`
	Result *MultiConserve `json:"result,omitempty"`
	DriftProof *BookSnapDriftProof `json:"drift_proof,omitempty"`
	Reused     bool                `json:"reused"`
}

type ProofExportReq struct {
	LeaseID string `json:"lease_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Limit int `json:"limit,omitempty"`
}

type ProofExportResp struct {
	Proofs []BookSnapDriftProof `json:"proofs"`
	Count  int                  `json:"count"`
}

type LeaseQueryReq struct {
	UserID string `json:"user_id,omitempty"`
	Asset  string `json:"asset,omitempty"`
	State string `json:"state,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type LeaseQueryResp struct {
	Leases []BookSnapLease `json:"leases"`
	Count int `json:"count"`
}

type RePinReq struct {
	LeaseID string `json:"lease_id"`
}

type RePinResp struct {
	LeaseID   string     `json:"lease_id"`
	SnapGen   int64      `json:"snap_gen"`
	State     LeaseState `json:"state"`
	BookSnaps []BookSnap `json:"book_snaps"`
	ExpiresAt time.Time `json:"expires_at"`
	RepinCount int `json:"repin_count"`
}

type AbortReq struct {
	LeaseID string `json:"lease_id"`
	Reason string `json:"reason,omitempty"`
}

type HealthResp struct {
	ActiveLeases int `json:"active_leases"`
	SealedLeases int `json:"sealed_leases"`
	DriftedLeases    int `json:"drifted_leases"`
	TotalDriftProofs int `json:"total_drift_proofs"`
}
