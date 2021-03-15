package brmismatch

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("brmismatch service unavailable")
	ErrAsset = fmt.Errorf("brmismatch asset invalid (allowed: USDT, USDC)")
	ErrActor = fmt.Errorf("brmismatch actor invalid")
	ErrKind  = fmt.Errorf("brmismatch kind invalid")
	ErrAmount   = fmt.Errorf("brmismatch amount invalid")
	ErrNotFound = fmt.Errorf("brmismatch case not found")
	ErrState    = fmt.Errorf("brmismatch case state transition invalid")
	ErrSuspended = fmt.Errorf("brmismatch account suspended; clear four-eyes first")
	ErrFourEyes  = fmt.Errorf("brmismatch four-eyes requires two distinct actors")
	ErrDuplicate = fmt.Errorf("brmismatch case already exists for this event pair")
	ErrProof     = fmt.Errorf("brmismatch proof hash computation failed")
	ErrWindow    = fmt.Errorf("brmismatch window key invalid")
	ErrClearDenied = fmt.Errorf("brmismatch case not in suspended state; cannot clear")
	ErrSuspendState = fmt.Errorf("brmismatch case must be in open state to suspend")
)
