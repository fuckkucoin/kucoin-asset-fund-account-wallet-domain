package usageleasegate

import (
	"errors"
	"fmt"
)

var (
	ErrNilEngine        = errors.New("usageleasegate: engine is nil")
	ErrStoreUnavailable = errors.New("usageleasegate: store unavailable")
	ErrUsageUnavailable = errors.New("usageleasegate: usage store unavailable")
	ErrUserRequired  = errors.New("usageleasegate: user required")
	ErrAssetRequired = errors.New("usageleasegate: asset required")
	ErrKindRequired  = errors.New("usageleasegate: occupancy kind required")
	ErrKindInvalid   = errors.New("usageleasegate: occupancy kind invalid")
	ErrAmountInvalid = errors.New("usageleasegate: amount must be positive")
	ErrLeaseCommitted = errors.New("usageleasegate: usage lease already committed")
	ErrLeaseNotReserved   = errors.New("usageleasegate: write requires reserved occupancy; only reserved writers may commit")
	ErrReservationMissing = errors.New("usageleasegate: occupancy reservation missing")
	ErrReservationExpired = errors.New("usageleasegate: occupancy reservation expired; auto-released")
	ErrReservationHeld = errors.New("usageleasegate: occupancy reservation already held by another writer")
	ErrWindowDrift    = errors.New("usageleasegate: occupancy window fingerprint drifted")
	ErrWouldExceedCap = errors.New("usageleasegate: write would exceed daily/velocity cap")
	ErrAlreadyBreached = errors.New("usageleasegate: daily cap already breached at snapshot")
	ErrMissingLease = errors.New("usageleasegate: record without usage lease fail-closed")
	ErrOwnerMismatch = errors.New("usageleasegate: lease owner mismatch")
	ErrBindingMismatch     = errors.New("usageleasegate: lease kind/user/asset/dest binding mismatch")
	ErrIllegalTransition   = errors.New("usageleasegate: illegal lease state transition")
	ErrProofNotFound       = errors.New("usageleasegate: drift proof not found")
	ErrTTLInvalid          = errors.New("usageleasegate: ttl must be positive")
	ErrVelocityInvalid     = errors.New("usageleasegate: velocity cap invalid")
	ErrHitstoreUnavailable = errors.New("usageleasegate: hitstore unavailable")
	ErrIdemConflict = errors.New("usageleasegate: idempotency key bound to another lease")
	ErrOpsReleaseDenied = errors.New("usageleasegate: ops release denied for committed occupancy")
	ErrAuditUnavailable = errors.New("usageleasegate: lease audit unavailable")
)

  func wrapDrift(kind DriftKind, err error) error {
	   if err == nil {
		  return fmt.Errorf("usageleasegate: drift %s", kind)
	  }
	  return fmt.Errorf("%w: %s", err, kind)
   }
