package incdef

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("incident defense unavailable")
	ErrActor       = fmt.Errorf("incident defense actor invalid")
	ErrKilled      = fmt.Errorf("incident kill switch on")
	ErrDegraded    = fmt.Errorf("incident degraded mode")
	ErrFrozen      = fmt.Errorf("incident auto freeze")
	ErrCircuit     = fmt.Errorf("incident circuit open")
	ErrDrift = fmt.Errorf("incident ledger drift")
	ErrStuck = fmt.Errorf("incident stuck tx")
	ErrAlert = fmt.Errorf("incident alert invalid")
	ErrHook = fmt.Errorf("incident hook refused")
)
