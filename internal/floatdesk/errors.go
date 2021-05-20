package floatdesk

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("float desk unavailable")
	ErrTarget      = fmt.Errorf("float target invalid")
	ErrChain       = fmt.Errorf("float chain invalid")
	ErrAsset = fmt.Errorf("float asset invalid")
	ErrHalt     = fmt.Errorf("float emergency halt")
	ErrSample   = fmt.Errorf("float sample invalid")
	ErrGas      = fmt.Errorf("gas tank invalid")
	ErrPlan     = fmt.Errorf("float plan refused")
	ErrNotFound = fmt.Errorf("float row not found")
)
