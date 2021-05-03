package dustleak

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("dustleak: service unavailable")
	ErrUser   = fmt.Errorf("dustleak: user_id invalid")
	ErrAsset  = fmt.Errorf("dustleak: asset invalid")
	ErrAmount = fmt.Errorf("dustleak: amount invalid")
	ErrPolicy = fmt.Errorf("dustleak: policy not found")
	ErrGenMismatch = fmt.Errorf("dustleak: policy generation changed during convert")
	ErrFloor       = fmt.Errorf("dustleak: amount is not below floor — not dust")
	ErrDriftExceed = fmt.Errorf("dustleak: quote drift exceeded cap — convert rejected")
	ErrBlocked = fmt.Errorf("dustleak: convert blocked — open leak case on this user+asset")
	ErrNotFound = fmt.Errorf("dustleak: record not found")
	ErrConflict = fmt.Errorf("dustleak: idempotency conflict")
	ErrState    = fmt.Errorf("dustleak: invalid state transition")
	ErrProof    = fmt.Errorf("dustleak: proof hash mismatch")
	ErrDual = fmt.Errorf("dustleak: dual approval required")
	ErrSnapshot     = fmt.Errorf("dustleak: quote snapshot invalid")
	ErrConserve     = fmt.Errorf("dustleak: conserve assertion failed")
	ErrActorInvalid = fmt.Errorf("dustleak: actor invalid")
)
