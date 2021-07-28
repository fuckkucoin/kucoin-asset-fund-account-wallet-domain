package invcheck

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("inventory check unavailable")
	ErrAsset    = fmt.Errorf("inventory check asset invalid")
	ErrAmount   = fmt.Errorf("inventory check amount invalid")
	ErrMismatch = fmt.Errorf("inventory check books do not match")
	ErrImbalance = fmt.Errorf("inventory check hot+cold does not conserve")
	ErrMissing = fmt.Errorf("inventory check missing book")
	ErrActor   = fmt.Errorf("inventory check actor invalid")
	ErrNotFound = fmt.Errorf("inventory check report not found")
	ErrStore = fmt.Errorf("inventory check store invalid")
)
