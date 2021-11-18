package riskscore

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("risk score unavailable")
	ErrUser        = fmt.Errorf("risk score user invalid")
	ErrChain       = fmt.Errorf("risk score chain invalid")
	ErrDest  = fmt.Errorf("risk score dest invalid")
	ErrAsset = fmt.Errorf("risk score asset invalid")
	ErrAmount = fmt.Errorf("risk score amount invalid")
	ErrEvent  = fmt.Errorf("risk score event invalid")
	ErrHalt   = fmt.Errorf("risk score halted")
	ErrMode = fmt.Errorf("risk score mode invalid")
)
