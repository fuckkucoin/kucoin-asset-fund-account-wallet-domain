package subacct

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("sub-account unavailable")
	ErrUser        = fmt.Errorf("sub-account user invalid")
	ErrParent      = fmt.Errorf("sub-account parent invalid")
	ErrChild       = fmt.Errorf("sub-account child invalid")
	ErrLabel       = fmt.Errorf("sub-account label invalid")
	ErrStatus      = fmt.Errorf("sub-account status refuses")
	ErrFrozen      = fmt.Errorf("sub-account frozen")
	ErrPrivilege   = fmt.Errorf("sub-account privilege missing")
	ErrAmount      = fmt.Errorf("sub-account amount invalid")
	ErrAsset       = fmt.Errorf("sub-account asset invalid")
	ErrIdem        = fmt.Errorf("sub-account idempotency conflict")
	ErrHop         = fmt.Errorf("sub-account hop refused")
	ErrBook        = fmt.Errorf("sub-account book invalid")
	ErrVelocity    = fmt.Errorf("sub-account velocity exceeded")
	ErrFence       = fmt.Errorf("sub-account fence stale")
	ErrClosed      = fmt.Errorf("sub-account closed")
	ErrSelf        = fmt.Errorf("sub-account self hop refused")
	ErrActor       = fmt.Errorf("sub-account actor invalid")
	ErrFourEye     = fmt.Errorf("sub-account grant requires a distinct reviewer")
	ErrCap         = fmt.Errorf("sub-account privilege cap exceeded")
	ErrScope       = fmt.Errorf("sub-account asset not in scope")
)
