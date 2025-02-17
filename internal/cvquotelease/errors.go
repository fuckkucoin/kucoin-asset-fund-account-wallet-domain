package cvquotelease

import (
	"errors"
	"fmt"
)

var (
	ErrNilEngine = errors.New("cvquotelease: engine is nil")
	ErrStoreUnavailable = errors.New("cvquotelease: store unavailable")
	ErrUserRequired  = errors.New("cvquotelease: user required")
	ErrAssetRequired = errors.New("cvquotelease: from/to assets required")
	ErrSameAsset     = errors.New("cvquotelease: from and to must differ")
	ErrAmountInvalid = errors.New("cvquotelease: amount must be positive")
	ErrRateInvalid   = errors.New("cvquotelease: rate must be positive")
	ErrBookInvalid   = errors.New("cvquotelease: book must be funding|trading|both")
	ErrQuoteSrc      = errors.New("cvquotelease: mark source unavailable")
	ErrLeaseRequired = errors.New("cvquotelease: quote lease required; strict bare place fail-closed")
	ErrLeaseNotFound = errors.New("cvquotelease: quote lease not found")
	ErrLeaseExpired  = errors.New("cvquotelease: quote lease expired")
	ErrLeaseRevoked  = errors.New("cvquotelease: quote lease revoked after mark generation bump")
	ErrLeaseConsumed = errors.New("cvquotelease: quote lease already consumed")
	ErrLeaseMismatch = errors.New("cvquotelease: lease binding mismatch")
	ErrOwnerMismatch    = errors.New("cvquotelease: quote owner mismatch")
	ErrFingerprintDrift = errors.New("cvquotelease: rate/book/mark fingerprint drifted")
	ErrMarkGenDrift     = errors.New("cvquotelease: mark_gen drifted after quote")
	ErrRateDrift = errors.New("cvquotelease: board rate drifted after quote")
	ErrBookDrift       = errors.New("cvquotelease: book fingerprint drifted after quote")
	ErrQuoteUsedRevive = errors.New("cvquotelease: expired quote cannot revive by Used=false")
	ErrDualExecute      = errors.New("cvquotelease: concurrent dual execute; lease already consumed")
	ErrProofNotFound    = errors.New("cvquotelease: drift proof not found")
	ErrMarkPairRequired = errors.New("cvquotelease: mark pair required")
	ErrIllegalTransition = errors.New("cvquotelease: illegal lease state transition")
	ErrFillHook        = errors.New("cvquotelease: fill hook rejected")
	ErrQuoteIDRequired = errors.New("cvquotelease: quote id required")
	ErrTTLInvalid      = errors.New("cvquotelease: ttl must be positive")
	ErrStrictBarePlace = errors.New("cvquotelease: strict mode rejects bare Place without lease")
	ErrQuoteNotFound  = errors.New("cvquotelease: quote snapshot not found")
	ErrMarkGenMissing = errors.New("cvquotelease: mark generation row missing; fail-closed")
)

  func wrapDrift(kind DriftKind, err error) error {
	   if err == nil {
		   return fmt.Errorf("cvquotelease: drift %s", kind)
	   }
	   return fmt.Errorf("%w: %s", err, kind)
   }
