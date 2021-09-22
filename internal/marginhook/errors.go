package marginhook

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("margin hook unavailable")
	ErrNotFound    = fmt.Errorf("margin hold not found")
	ErrLTV         = fmt.Errorf("margin ltv breached")
	ErrClosed      = fmt.Errorf("margin hold closed")
	ErrAsset       = fmt.Errorf("collateral asset mismatch")
	ErrUser = fmt.Errorf("user_id required")
	ErrLiqGen = fmt.Errorf("margin liq_gen fence")
	ErrLiqMutex = fmt.Errorf("margin freeze/release mutex")
	ErrLiqHalf  = fmt.Errorf("margin freeze half-open")
)
