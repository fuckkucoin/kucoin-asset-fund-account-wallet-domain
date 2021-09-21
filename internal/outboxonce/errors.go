package outboxonce

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("outbox unavailable")
	ErrKey  = fmt.Errorf("outbox key invalid")
	ErrKind = fmt.Errorf("outbox kind invalid")
	ErrPayload = fmt.Errorf("outbox payload invalid")
	ErrDup     = fmt.Errorf("outbox key already claimed")
	ErrFence   = fmt.Errorf("outbox fence stale")
	ErrHalt      = fmt.Errorf("outbox halted")
	ErrActor     = fmt.Errorf("outbox actor invalid")
	ErrDelivered = fmt.Errorf("outbox already delivered")
	ErrConflict = fmt.Errorf("outbox payload conflict")
	// halt_gen 互斥扩展错误
	ErrHaltGenMismatch = fmt.Errorf("outbox halt_gen mismatch: deliver bound to stale generation")
	ErrHaltGenRevoked = fmt.Errorf("outbox halt_gen revoked: in-flight deliver revoked by halt")
	ErrHaltGenCAS       = fmt.Errorf("outbox halt_gen CAS failed: concurrent halt state change")
	ErrStaleFenceRevive = fmt.Errorf("outbox stale fence revive blocked: epoch/token revoked during halt")
	ErrDoubleDeliver    = fmt.Errorf("outbox double deliver detected")
	ErrDoubleMark = fmt.Errorf("outbox double mark detected")
	ErrProofNotFound = fmt.Errorf("outbox proof not found")
	ErrLeaseNotFound = fmt.Errorf("outbox halt lease not found")
)
