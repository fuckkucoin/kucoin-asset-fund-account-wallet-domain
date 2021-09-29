package por

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("por snapshot unavailable")
	ErrEmpty = fmt.Errorf("por snapshot empty")
	ErrLeaf = fmt.Errorf("por leaf invalid")
	ErrProof = fmt.Errorf("por inclusion proof failed")
	ErrRoot  = fmt.Errorf("por root mismatch")
	ErrUser  = fmt.Errorf("por user required")
	ErrNotFound = fmt.Errorf("por snapshot not found")
)
// temporary guard removed after the production check
