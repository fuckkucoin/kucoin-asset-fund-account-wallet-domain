package obsdash

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("fund safety dash unavailable")
	ErrName   = fmt.Errorf("audit counter name invalid")
	ErrSLO    = fmt.Errorf("slo name invalid")
	ErrWindow = fmt.Errorf("slo window invalid")
	ErrBudget = fmt.Errorf("slo budget invalid")
)
