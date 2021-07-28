package invdrift

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("invdrift service unavailable")
	ErrStore       = fmt.Errorf("invdrift store not configured")
	ErrBatchNotFound = fmt.Errorf("invdrift batch not found")
	ErrItemNotFound   = fmt.Errorf("invdrift item not found")
	ErrAdviceNotFound = fmt.Errorf("invdrift advice not found")
	ErrProofNotFound = fmt.Errorf("invdrift proof not found")
	ErrBadState = fmt.Errorf("invdrift invalid state transition")
	ErrAsset = fmt.Errorf("invdrift asset invalid")
	ErrAmount       = fmt.Errorf("invdrift amount invalid")
	ErrActor        = fmt.Errorf("invdrift actor invalid")
	ErrAdviceActive = fmt.Errorf("invdrift active seal advice exists")
	ErrIntegrity = fmt.Errorf("invdrift proof integrity check failed")
	ErrConflict  = fmt.Errorf("invdrift concurrent batch in progress")
)
