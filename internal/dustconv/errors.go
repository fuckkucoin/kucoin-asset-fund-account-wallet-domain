package dustconv

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("dust converter unavailable")
	ErrUser = fmt.Errorf("dust user invalid")
	ErrAsset = fmt.Errorf("dust asset invalid")
	ErrAmount = fmt.Errorf("dust amount invalid")
	ErrFloor = fmt.Errorf("dust floor invalid")
	ErrRate = fmt.Errorf("dust rate invalid")
	ErrPolicy = fmt.Errorf("dust policy refused")
	ErrIdem = fmt.Errorf("dust idempotency conflict")
	ErrActor    = fmt.Errorf("dust actor invalid")
	ErrConserve = fmt.Errorf("dust convert does not conserve")
	ErrNotDust  = fmt.Errorf("amount is not below floor")
)
