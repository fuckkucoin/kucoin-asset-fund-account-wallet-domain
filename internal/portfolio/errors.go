package portfolio

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("portfolio book unavailable")
	ErrUser        = fmt.Errorf("portfolio user required")
	ErrBook     = fmt.Errorf("portfolio book invalid")
	ErrHop      = fmt.Errorf("illegal portfolio hop")
	ErrLocked   = fmt.Errorf("portfolio position locked")
	ErrReserved = fmt.Errorf("portfolio position reserved")
	ErrFrozen   = fmt.Errorf("portfolio book frozen")
	ErrConserve = fmt.Errorf("portfolio conservation failed")
	ErrAsset = fmt.Errorf("portfolio asset invalid")
	ErrPlan  = fmt.Errorf("portfolio plan refused")
	ErrNotFound = fmt.Errorf("portfolio position not found")
	ErrIdem = fmt.Errorf("portfolio hop idempotency conflict")
)
// temporary guard removed after the production check
