package fundsafety

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("fund safety unavailable")
	ErrUser        = fmt.Errorf("fund safety user invalid")
	ErrAsset    = fmt.Errorf("fund safety asset invalid")
	ErrAmount   = fmt.Errorf("fund safety amount invalid")
	ErrHash     = fmt.Errorf("fund safety hash invalid")
	ErrDup      = fmt.Errorf("fund safety double credit")
	ErrNegative = fmt.Errorf("fund safety negative balance")
	ErrPartial  = fmt.Errorf("fund safety partial batch imbalance")
	ErrFrozen = fmt.Errorf("fund safety account frozen")
	ErrHalt   = fmt.Errorf("fund safety halted")
	ErrActor    = fmt.Errorf("fund safety actor invalid")
	ErrConserve = fmt.Errorf("fund safety conservation failed")
	ErrReorg    = fmt.Errorf("fund safety reorg replay refused")
	ErrFence    = fmt.Errorf("fund safety fence stale")
	ErrConfirm = fmt.Errorf("fund safety confirm window short")
	ErrDepth = fmt.Errorf("fund safety reorg depth refused")
	ErrBatch  = fmt.Errorf("fund safety batch refused")
	ErrOutbox = fmt.Errorf("fund safety outbox refused")
	ErrClosed = fmt.Errorf("fund safety batch closed")
)
