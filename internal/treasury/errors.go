package treasury

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("treasury book unavailable")
	ErrBand = fmt.Errorf("treasury band invalid")
	ErrAsset   = fmt.Errorf("treasury asset invalid")
	ErrConfirm = fmt.Errorf("cold receive not confirmed")
	ErrAlert   = fmt.Errorf("treasury imbalance")
	ErrPlan    = fmt.Errorf("sweep plan refused")
)
