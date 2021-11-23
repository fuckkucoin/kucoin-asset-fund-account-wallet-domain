package stablebr

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("stable bridge unavailable")
	ErrUser   = fmt.Errorf("stable bridge user invalid")
	ErrAsset  = fmt.Errorf("stable bridge asset invalid")
	ErrAmount = fmt.Errorf("stable bridge amount invalid")
	ErrIdem   = fmt.Errorf("stable bridge idempotency conflict")
	ErrActor  = fmt.Errorf("stable bridge actor invalid")
	ErrAttest = fmt.Errorf("stable bridge attestation incomplete")
	ErrExpired = fmt.Errorf("stable bridge attestation expired")
	ErrKind    = fmt.Errorf("stable bridge kind invalid")
	ErrConserve = fmt.Errorf("stable bridge journal does not conserve")
	ErrItem     = fmt.Errorf("stable bridge checklist item invalid")
	ErrNotFound = fmt.Errorf("stable bridge row not found")
	ErrState    = fmt.Errorf("stable bridge state invalid")
	ErrLease    = fmt.Errorf("stable bridge mint lease required")
	ErrHalfOpen = fmt.Errorf("stable bridge journal half-open, replay blocked")
	ErrFingerprint = fmt.Errorf("stable bridge attest fingerprint drift")
)
