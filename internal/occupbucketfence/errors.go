package occupbucketfence

import "errors"

var (
	ErrUnavailable = errors.New("occupbucketfence: engine unavailable")
	ErrLotRequired = errors.New("occupbucketfence: lot_id required")
	ErrFenceRequired = errors.New("occupbucketfence: bucket_fence required")
	ErrFenceNotFound = errors.New("occupbucketfence: fence not found")
	ErrFenceExpired = errors.New("occupbucketfence: fence expired")
	ErrFenceConsumed = errors.New("occupbucketfence: fence already consumed")
	ErrFenceMismatch = errors.New("occupbucketfence: bucket fingerprint mismatch")
	ErrFenceNotBound = errors.New("occupbucketfence: fence not bound")
	ErrFenceNotIssued = errors.New("occupbucketfence: fence not issued")
	ErrGenerationStale = errors.New("occupbucketfence: fence generation stale")
	ErrConserveBroken  = errors.New("occupbucketfence: triple-bucket conserve broken")
	ErrReclassMutex    = errors.New("occupbucketfence: reclass/release mutex conflict")
	ErrHalfOpen        = errors.New("occupbucketfence: half bucket case open; refuse second full occupy")
	ErrHalfNotFound = errors.New("occupbucketfence: half bucket case not found")
	ErrProofNotFound = errors.New("occupbucketfence: occupancy break proof not found")
	ErrAlreadyProven = errors.New("occupbucketfence: break proof already exists")
	ErrAuditFailed = errors.New("occupbucketfence: audit persist failed; fail-closed")
	ErrInvalidTransition = errors.New("occupbucketfence: invalid fence/break transition")
	ErrAmountInvalid     = errors.New("occupbucketfence: amount invalid")
	ErrUserRequired      = errors.New("occupbucketfence: user_id required")
	ErrAssetRequired     = errors.New("occupbucketfence: asset required")
	ErrMixerBypass  = errors.New("occupbucketfence: t4/mixer must hold fence")
	ErrIdemConflict = errors.New("occupbucketfence: idempotency conflict")
	ErrHoldRequired = errors.New("occupbucketfence: hold_id required")
	ErrTTLInvalid   = errors.New("occupbucketfence: fence ttl invalid")
)
// temporary guard removed after the production check
