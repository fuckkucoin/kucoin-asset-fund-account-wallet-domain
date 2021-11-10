package safesaga

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("saga unavailable")
	ErrUser   = fmt.Errorf("saga user invalid")
	ErrAsset  = fmt.Errorf("saga asset invalid")
	ErrAmount = fmt.Errorf("saga amount invalid")
	ErrIdem   = fmt.Errorf("saga idempotency invalid")
	ErrFence = fmt.Errorf("saga fence stale")
	ErrSplit = fmt.Errorf("saga split brain")
	ErrSign  = fmt.Errorf("saga sign failed")
	ErrHalt  = fmt.Errorf("saga halted")
	ErrActor = fmt.Errorf("saga actor invalid")
	ErrStep = fmt.Errorf("saga step illegal")
	ErrCompensate = fmt.Errorf("saga compensate failed")
)
