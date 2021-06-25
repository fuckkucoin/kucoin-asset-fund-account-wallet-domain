package fxmarkleasegate

import (
	"errors"
	"fmt"
)

var (
	ErrNilEngine        = errors.New("fxmarkleasegate: engine is nil")
	ErrStoreUnavailable = errors.New("fxmarkleasegate: store unavailable")
	ErrBookUnavailable = errors.New("fxmarkleasegate: fx mark book unavailable")
	ErrPairRequired = errors.New("fxmarkleasegate: from/to pair required")
	ErrPairInvalid  = errors.New("fxmarkleasegate: from/to pair invalid")
	ErrRateMissing  = errors.New("fxmarkleasegate: rate missing; cannot issue post lease")
	ErrRateInvalid  = errors.New("fxmarkleasegate: rate invalid")
	ErrLeaseRequired = errors.New("fxmarkleasegate: mark_lease_id required; fail-closed")
	ErrLeaseNotFound = errors.New("fxmarkleasegate: mark lease not found")
	ErrLeaseExpired = errors.New("fxmarkleasegate: mark lease expired")
	ErrLeaseRevoked = errors.New("fxmarkleasegate: mark lease revoked after put/live drift")
	ErrLeaseConsumed  = errors.New("fxmarkleasegate: mark lease already consumed")
	ErrLeaseNotBound  = errors.New("fxmarkleasegate: post requires bound mark lease")
	ErrLeaseNotIssued = errors.New("fxmarkleasegate: lease not in issued state")
	ErrFingerprintDrift = errors.New("fxmarkleasegate: mark source fingerprint drifted")
	ErrLiveOverThreshold = errors.New("fxmarkleasegate: live feed drift over threshold")
	ErrPutInvalidated  = errors.New("fxmarkleasegate: put price change invalidated lease")
	ErrGenerationDrift = errors.New("fxmarkleasegate: mark generation drifted")
	ErrMissingRate     = errors.New("fxmarkleasegate: missing rate fail-closed")
	ErrMissingLease = errors.New("fxmarkleasegate: post without mark lease fail-closed")
	ErrBindingMismatch   = errors.New("fxmarkleasegate: lease pair/source binding mismatch")
	ErrIllegalTransition = errors.New("fxmarkleasegate: illegal lease state transition")
	ErrProofNotFound     = errors.New("fxmarkleasegate: drift proof not found")
	ErrTTLInvalid        = errors.New("fxmarkleasegate: ttl must be positive")
	ErrThresholdInvalid = errors.New("fxmarkleasegate: drift threshold invalid")
	ErrOpsRevokeDenied = errors.New("fxmarkleasegate: ops revoke denied for consumed lease")
	ErrIdemConflict    = errors.New("fxmarkleasegate: idempotency key bound to another lease")
	ErrActorRequired   = errors.New("fxmarkleasegate: ops actor required")
	ErrPurposeInvalid  = errors.New("fxmarkleasegate: post purpose invalid")
	ErrAuditUnavailable = errors.New("fxmarkleasegate: mark lease audit unavailable")
	ErrLiveUnavailable = errors.New("fxmarkleasegate: live feed unavailable for recheck")
)

  func wrapDrift(kind DriftKind, err error) error {
	  if err == nil {
		   return fmt.Errorf("fxmarkleasegate: drift %s", kind)
	   }
	   return fmt.Errorf("%w: %s", err, kind)
   }
