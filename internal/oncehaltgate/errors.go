package oncehaltgate

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("oncehaltgate unavailable")
	ErrHalt        = fmt.Errorf("oncehaltgate halted: deliver rejected")
	ErrHaltGenCAS  = fmt.Errorf("oncehaltgate halt_gen CAS failed: generation changed during deliver")
	ErrHaltGenRevoked = fmt.Errorf("oncehaltgate deliver revoked by halt transition")
	ErrStaleFence    = fmt.Errorf("oncehaltgate stale fence: epoch/token revoked during halt, revival blocked")
	ErrDoubleDeliver = fmt.Errorf("oncehaltgate double deliver detected")
	ErrNoService     = fmt.Errorf("oncehaltgate: outboxonce service not bound")
	ErrBadRequest    = fmt.Errorf("oncehaltgate bad request")
)
