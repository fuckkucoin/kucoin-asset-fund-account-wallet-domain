package allowleasegate

import "fmt"

var (
	ErrLeaseNotFound = fmt.Errorf("allow_lease not found")
	ErrLeaseExpired = fmt.Errorf("allow_lease expired; withdraw stays fail-closed")
	ErrLeaseRevoked = fmt.Errorf("allow_lease revoked; case denied or expired")
	ErrLeaseConsumed = fmt.Errorf("allow_lease already consumed")
	ErrLeaseIllegal = fmt.Errorf("illegal lease state transition")
	ErrNoLease          = fmt.Errorf("no allow_lease; broadcast blocked")
	ErrCaseMismatch     = fmt.Errorf("lease case_id mismatch")
	ErrPolicyMismatch   = fmt.Errorf("lease policy_version mismatch")
	ErrMovementMismatch = fmt.Errorf("lease movement_id mismatch")
	ErrCaseNotAllowed = fmt.Errorf("case state does not permit allow_lease issuance")
	ErrCaseDenied     = fmt.Errorf("case denied; lease revoked")
	ErrCaseExpired    = fmt.Errorf("case expired; lease revoked")
	ErrAlreadyBound = fmt.Errorf("lease already bound to a different movement")
	ErrEngineNil    = fmt.Errorf("allow_lease engine unavailable")
	ErrProofSealed  = fmt.Errorf("drift proof already sealed")
	ErrStoreNil     = fmt.Errorf("allow_lease store unavailable")
	ErrInvalidTTL   = fmt.Errorf("invalid lease TTL")
	ErrBindConflict = fmt.Errorf("lease bind conflict; concurrent submit race")
)
