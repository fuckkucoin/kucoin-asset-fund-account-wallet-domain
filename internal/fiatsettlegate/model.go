package fiatsettlegate

import "time"

// LeaseState 结清租约状态。

type LeaseState string

const (
	LeaseAuthorized LeaseState = "authorized"
	LeaseSettleBound LeaseState = "settle_bound"
	LeaseSettled     LeaseState = "settled"
	LeaseRefunding   LeaseState = "refunding"
	LeaseRefunded LeaseState = "refunded"
	LeaseCompensate LeaseState = "compensate"
	LeaseRevoked    LeaseState = "revoked"
	LeaseExpired LeaseState = "expired"
)

// Terminal 终态不可再绑定 Capture。

   func (s LeaseState) Terminal() bool {
	  return s == LeaseSettled || s == LeaseRefunded || s == LeaseRevoked || s == LeaseExpired
   }

// OpenForSettle 是否允许进入 Settle 绑定。
   func (s LeaseState) OpenForSettle() bool {
	   return s == LeaseAuthorized || s == LeaseCompensate
   }

// OpenForRefund 是否允许进入退款互斥。
  func (s LeaseState) OpenForRefund() bool {
	   return s == LeaseAuthorized || s == LeaseSettled
   }

// FiatSettleLease Authorize 世代钉扎。
// 字段含 booking_id、PSPIntent、KYCTier、Limits 指纹、TTL。
// 对外只读标识：fiat_settle_lease_id。
type FiatSettleLease struct {
	LeaseID   string `json:"fiat_settle_lease_id"`
	BookingID string `json:"booking_id"`
	UserID    string `json:"user_id"`
	Kind      string `json:"kind"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Currency          string `json:"currency"`
	PSPIntent         string `json:"psp_intent"`
	KYCTier           string `json:"kyc_tier"`
	KYCFingerprint    string `json:"kyc_fingerprint"`
	LimitsFingerprint string `json:"limits_fingerprint"`
	ReservationID     string `json:"limits_reservation_id,omitempty"`
	CaptureRef string `json:"capture_ref,omitempty"`
	CaptureSealed bool       `json:"capture_sealed"`
	LedgerID      string     `json:"ledger_id,omitempty"`
	State         LeaseState `json:"state"`
	Generation int64 `json:"generation"`
	TTLSeconds int64 `json:"ttl_seconds"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	BoundAt   *time.Time `json:"bound_at,omitempty"`
	SettledAt *time.Time `json:"settled_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	Actor     string     `json:"actor,omitempty"`
	Note             string `json:"note,omitempty"`
	CompensateReason string `json:"compensate_reason,omitempty"`
}

// Copy 值拷贝。
   func (l FiatSettleLease) Copy() FiatSettleLease { return l }

// Alive 未过期且非终态。
   func (l FiatSettleLease) Alive(now time.Time) bool {
	   if l.State.Terminal() {
		   return false
	  }
	   if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) && l.State == LeaseAuthorized {
		  return false
	   }
	   return true
   }

// ReservationState 限额占用状态。

type ReservationState string

const (
	ResReserved ReservationState = "reserved"
	ResRecorded ReservationState = "recorded"
	ResReleased ReservationState = "released"
	ResExpired  ReservationState = "expired"
)

// LimitsReservation Book/Authorize Check 后占用，Record 仅 Settle 成功。

type LimitsReservation struct {
	ReservationID string `json:"limits_reservation_id"`
	BookingID     string `json:"booking_id"`
	UserID string `json:"user_id"`
	Kind   string `json:"kind"`
	Amount string `json:"amount"`
	LimitsFingerprint string           `json:"limits_fingerprint"`
	State             ReservationState `json:"state"`
	LeaseID string `json:"fiat_settle_lease_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	RecordedAt    *time.Time `json:"recorded_at,omitempty"`
	ReleasedAt    *time.Time `json:"released_at,omitempty"`
	ReleaseReason string     `json:"release_reason,omitempty"`
}

// RefundLeaseState 退款租约状态。
type RefundLeaseState string

const (
	RefundIssued    RefundLeaseState = "issued"
	RefundBound     RefundLeaseState = "bound"
	RefundCompleted RefundLeaseState = "completed"
	RefundFailed  RefundLeaseState = "failed"
	RefundBlocked RefundLeaseState = "blocked"
)

// RefundLease 已 Settled 退款绑定反记账与 PSP Refund；与在途 Settle 互斥。

type RefundLease struct {
	RefundLeaseID string `json:"refund_lease_id"`
	SettleLeaseID string `json:"fiat_settle_lease_id"`
	BookingID     string `json:"booking_id"`
	PSPIntent     string `json:"psp_intent"`
	ReverseLedger string           `json:"reverse_ledger_id,omitempty"`
	PSPRefundRef  string           `json:"psp_refund_ref,omitempty"`
	State         RefundLeaseState `json:"state"`
	Actor    string    `json:"actor"`
	Reason   string    `json:"reason"`
	IssuedAt time.Time `json:"issued_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
}

// ProofKind 证明类型。
type ProofKind string

const (
	ProofLimitsDrift ProofKind = "limits_fingerprint_drift"
	ProofKYCDowngrade ProofKind = "kyc_fingerprint_downgrade"
	ProofPSPMismatch  ProofKind = "psp_intent_mismatch"
	ProofSettleRefundMux ProofKind = "settle_refund_mutex"
	ProofCaptureOrphan   ProofKind = "capture_ok_ledger_fail"
	ProofDoubleCapture ProofKind = "second_capture_blocked"
	ProofExpireRevoke  ProofKind = "lease_expired_revoked"
	ProofOverLimitOccupy ProofKind = "limits_over_occupy_patrol"
	ProofIdemReplay      ProofKind = "idempotent_replay"
	ProofCompensateRetry ProofKind = "compensate_retry_no_recapture"
)

// FiatSettleProof 可导出结清证明。
type FiatSettleProof struct {
	ProofID string `json:"proof_id"`
	LeaseID   string `json:"fiat_settle_lease_id"`
	BookingID string `json:"booking_id"`
	Kind       ProofKind `json:"kind"`
	FailClosed bool      `json:"fail_closed"`
	Winner string `json:"winner,omitempty"`
	Loser string `json:"loser,omitempty"`
	Summary string `json:"summary"`
	CaptureSealed bool      `json:"capture_sealed"`
	SettledHeld   bool      `json:"settled_held"`
	OpenedAt      time.Time `json:"opened_at"`
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	Detail     string     `json:"detail,omitempty"`
}

// AuditEvent 结清审计事件。

type AuditEvent struct {
	AuditID   string `json:"audit_id"`
	LeaseID   string `json:"fiat_settle_lease_id,omitempty"`
	BookingID string `json:"booking_id"`
	Action    string `json:"action"`
	Actor     string `json:"actor"`
	FromState string `json:"from_state,omitempty"`
	ToState string `json:"to_state,omitempty"`
	Detail string `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthorizeLeaseRequest AuthorizeWithLease 请求。
type AuthorizeLeaseRequest struct {
	BookingID string `json:"booking_id"`
	Actor string `json:"actor"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
	IdemKey    string `json:"idem_key,omitempty"`
}

// SettleAssertRequest SettleAssert 请求。必须携带未过期 settle_lease。

type SettleAssertRequest struct {
	LeaseID   string `json:"fiat_settle_lease_id"`
	BookingID string `json:"booking_id"`
	Actor     string `json:"actor"`
	IdemKey string `json:"idem_key,omitempty"`
}

// RefundWithLeaseRequest RefundWithLease 请求。
type RefundWithLeaseRequest struct {
	BookingID string `json:"booking_id"`
	LeaseID string `json:"fiat_settle_lease_id,omitempty"`
	Actor   string `json:"actor"`
	Reason  string `json:"reason"`
	IdemKey string `json:"idem_key,omitempty"`
}

// OccupyLimitsRequest 限额占用请求。

type OccupyLimitsRequest struct {
	BookingID string `json:"booking_id"`
	UserID    string `json:"user_id"`
	Kind   string `json:"kind"`
	Amount string `json:"amount"`
	CardRef string `json:"card_ref,omitempty"`
}

// WorkerTickResult 巡检结果。
type WorkerTickResult struct {
	ExpiredRevoked int `json:"expired_revoked"`
	OrphansScanned int `json:"orphans_scanned"`
	OrphansPumped int `json:"orphans_pumped"`
	OverLimitHits int `json:"over_limit_hits"`
	ReservReleased int `json:"reservations_released"`
}

// LeaseView 只读视图。
type LeaseView struct {
	Lease *FiatSettleLease `json:"lease,omitempty"`
	Reservation *LimitsReservation `json:"reservation,omitempty"`
	Refund *RefundLease      `json:"refund_lease,omitempty"`
	Proofs []FiatSettleProof `json:"proofs,omitempty"`
	Audits []AuditEvent      `json:"audits,omitempty"`
}

// DefaultLeaseTTL 授权租约默认存活。
const DefaultLeaseTTL = int64(900)
