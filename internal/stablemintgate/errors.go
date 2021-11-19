package stablemintgate

import "fmt"

var (
	ErrUnavailable   = fmt.Errorf("stablemintgate: engine unavailable")
	ErrInvalidInput  = fmt.Errorf("stablemintgate: invalid input")
	ErrLeaseNotFound = fmt.Errorf("stablemintgate: mint_lease not found")
	ErrLeaseExpired  = fmt.Errorf("stablemintgate: mint_lease expired")
	ErrLeaseRevoked  = fmt.Errorf("stablemintgate: mint_lease revoked")
	ErrLeaseConsumed = fmt.Errorf("stablemintgate: mint_lease already consumed")
	ErrLeasePosting      = fmt.Errorf("stablemintgate: mint_lease posting in progress")
	ErrIllegalTransition = fmt.Errorf("stablemintgate: illegal lease transition")
	ErrNoLease          = fmt.Errorf("stablemintgate: mint/redeem requires mint_lease_id")
	ErrAttestIncomplete = fmt.Errorf("stablemintgate: attestation incomplete")
	ErrAttestExpired    = fmt.Errorf("stablemintgate: attestation expired")
	ErrFingerprintDrift = fmt.Errorf("stablemintgate: attest fingerprint drift")
	ErrGenMismatch      = fmt.Errorf("stablemintgate: mint generation mismatch")
	ErrKindMismatch     = fmt.Errorf("stablemintgate: lease kind mismatch")
	ErrAssetMismatch = fmt.Errorf("stablemintgate: lease asset mismatch")
	ErrActorMismatch = fmt.Errorf("stablemintgate: lease actor mismatch")
	ErrIdemReplay = fmt.Errorf("stablemintgate: idempotency replay blocked second mint")
	ErrHalfOpen   = fmt.Errorf("stablemintgate: journal half-open, second mint denied")
	ErrHalfNotFound  = fmt.Errorf("stablemintgate: half proof not found")
	ErrProofNotFound = fmt.Errorf("stablemintgate: mint proof not found")
	ErrBridge = fmt.Errorf("stablemintgate: stablebr reject")
	ErrLedger = fmt.Errorf("stablemintgate: ledger create failed")
	ErrAmount = fmt.Errorf("stablemintgate: amount invalid")
)
