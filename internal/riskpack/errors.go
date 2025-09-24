package riskpack

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("risk pack unavailable")
	ErrUser = fmt.Errorf("user_id required")
	ErrCap  = fmt.Errorf("exposure cap breached")
	ErrCluster = fmt.Errorf("withdraw cluster breached")
	ErrDest     = fmt.Errorf("correlated destination held")
	ErrEscalate = fmt.Errorf("manual escalate required")
	ErrNotFound = fmt.Errorf("escalate item not found")
	ErrClosed = fmt.Errorf("escalate item closed")
	ErrActor  = fmt.Errorf("risk actor required")
)
