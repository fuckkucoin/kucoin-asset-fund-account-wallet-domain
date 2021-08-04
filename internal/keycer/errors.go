package keycer

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("key ceremony unavailable")
	ErrSlot        = fmt.Errorf("hsm slot invalid")
	ErrState       = fmt.Errorf("hsm slot illegal state")
	ErrActor   = fmt.Errorf("key ceremony actor invalid")
	ErrTicket  = fmt.Errorf("unlock ticket not found")
	ErrFourEye = fmt.Errorf("unlock needs a second actor")
	ErrLabel  = fmt.Errorf("hsm slot label invalid")
	ErrRotate = fmt.Errorf("key rotation refused")
	ErrFingerprint = fmt.Errorf("hsm fingerprint invalid")
)
