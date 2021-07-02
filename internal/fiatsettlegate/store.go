package fiatsettlegate

import (
	"context"
	"time"
)

// Store 内存/SQL 共用契约。
type Store interface {
	InsertLease(ctx context.Context, l FiatSettleLease) error
	GetLease(ctx context.Context, leaseID string) (*FiatSettleLease, error)
	GetLeaseByBooking(ctx context.Context, bookingID string) (*FiatSettleLease, error)
	CASLease(ctx context.Context, leaseID string, expectedGen int64, expectedState LeaseState, next FiatSettleLease) error
	ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]FiatSettleLease, error)
	ListExpiredAuthorized(ctx context.Context, now time.Time, limit int) ([]FiatSettleLease, error)
	ListCompensate(ctx context.Context, limit int) ([]FiatSettleLease, error)
	InsertReservation(ctx context.Context, r LimitsReservation) error
	GetReservation(ctx context.Context, id string) (*LimitsReservation, error)
	GetReservationByBooking(ctx context.Context,
		bookingID string) (*LimitsReservation, error)
	CASReservation(ctx context.Context, id string, expected ReservationState, next LimitsReservation) error
	ListReservationsByState(ctx context.Context, state ReservationState, limit int) ([]LimitsReservation, error)
	ListOpenReservationsByUser(ctx context.Context,
		userID, kind string) ([]LimitsReservation, error)
	InsertRefundLease(ctx context.Context, r RefundLease) error
	GetRefundLease(ctx context.Context, id string) (*RefundLease, error)
	GetRefundLeaseByBooking(ctx context.Context,
		bookingID string) (*RefundLease, error)
	CASRefundLease(ctx context.Context, id string, expected RefundLeaseState, next RefundLease) error
	InsertProof(ctx context.Context, p FiatSettleProof) error
	GetProof(ctx context.Context,
		id string) (*FiatSettleProof, error)
	ListProofs(ctx context.Context, limit int) ([]FiatSettleProof, error)
	ListProofsByLease(ctx context.Context, leaseID string) ([]FiatSettleProof, error)
	MarkProofExported(ctx context.Context, id string, now time.Time) error
	InsertAudit(ctx context.Context, a AuditEvent) error
	ListAuditsByLease(ctx context.Context, leaseID string, limit int) ([]AuditEvent, error)
	ListAuditsByBooking(ctx context.Context, bookingID string, limit int) ([]AuditEvent, error)
	PutIdem(ctx context.Context,
		key, resultID string) error
	GetIdem(ctx context.Context, key string) (string, bool, error)
	// MutexToken 在途 Settle/Refund 互斥令牌。同一 booking 仅一方持有。
	TryLockBooking(ctx context.Context, bookingID, holder, purpose string, now time.Time, ttl time.Duration) error
	UnlockBooking(ctx context.Context, bookingID, holder string) error
	GetBookingLock(ctx context.Context, bookingID string) (holder, purpose string, ok bool)
	Snapshot() StoreSnapshot
	Restore(StoreSnapshot)
}

// BookingLock 在途互斥。

type BookingLock struct {
	BookingID string `json:"booking_id"`
	Holder    string `json:"holder"`
	Purpose string `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StoreSnapshot 重启复放。

type StoreSnapshot struct {
	Leases []FiatSettleLease `json:"leases"`
	Reservations []LimitsReservation `json:"reservations"`
	Refunds      []RefundLease       `json:"refunds"`
	Proofs       []FiatSettleProof   `json:"proofs"`
	Audits []AuditEvent      `json:"audits"`
	Idem   map[string]string `json:"idem"`
	Locks  []BookingLock     `json:"locks"`
}
