package sweeprun

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("sweep orchestrator unavailable")
	ErrJob         = fmt.Errorf("sweep job not found")
	ErrState = fmt.Errorf("sweep illegal state")
	ErrAmount = fmt.Errorf("sweep amount invalid")
	ErrBudget = fmt.Errorf("sweep budget exceeded")
	ErrChain = fmt.Errorf("sweep chain invalid")
	ErrAsset = fmt.Errorf("sweep asset invalid")
	ErrActor = fmt.Errorf("sweep actor invalid")
	ErrIdem       = fmt.Errorf("sweep idempotency conflict")
	ErrFill       = fmt.Errorf("sweep fill refused")
	ErrCap        = fmt.Errorf("sweep job cap exceeded")
	ErrAgreeLease = fmt.Errorf("sweep agree_lease_id required or refused")
	ErrAgreeRule  = fmt.Errorf("sweep agree rule snapshot missing")
	ErrAgreeCode = fmt.Errorf("sweep agree code failed")
)
