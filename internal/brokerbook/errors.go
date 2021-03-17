package brokerbook

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("broker book unavailable")
	ErrMaster      = fmt.Errorf("broker master invalid")
	ErrSub  = fmt.Errorf("broker sub-account invalid")
	ErrUser = fmt.Errorf("broker user invalid")
	ErrAsset  = fmt.Errorf("broker asset invalid")
	ErrAmount = fmt.Errorf("broker amount invalid")
	ErrActor  = fmt.Errorf("broker actor invalid")
	ErrIdem = fmt.Errorf("broker idempotency conflict")
	ErrPriv     = fmt.Errorf("broker privilege refused")
	ErrFence    = fmt.Errorf("broker sub-account fenced")
	ErrState    = fmt.Errorf("broker illegal state")
	ErrConserve = fmt.Errorf("broker books do not conserve")
	ErrLabel    = fmt.Errorf("broker label invalid")
)
