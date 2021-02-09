package amldesk

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("aml desk unavailable")
	ErrNotFound = fmt.Errorf("aml case not found")
	ErrClosed   = fmt.Errorf("aml case closed")
	ErrActor    = fmt.Errorf("aml actor required")
	ErrFourEye  = fmt.Errorf("four-eye close requires a distinct actor")
	ErrEvidence = fmt.Errorf("aml evidence invalid")
	ErrTravel = fmt.Errorf("travel-rule payload invalid")
	ErrKind   = fmt.Errorf("aml case kind invalid")
	ErrState = fmt.Errorf("illegal aml transition")
	ErrWatch = fmt.Errorf("aml watch subject invalid")
	ErrAssign = fmt.Errorf("aml reviewer invalid")
	ErrSLA    = fmt.Errorf("aml case overdue")
	ErrHoldBlocked    = fmt.Errorf("aml case resolution blocked by open quarantine hold")
	ErrMutexViolation = fmt.Errorf("aml release/close mutex violation: same actor detected")
)
