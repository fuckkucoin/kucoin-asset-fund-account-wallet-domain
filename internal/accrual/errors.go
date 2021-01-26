package accrual

import "fmt"

var (
	ErrEngineNil     = fmt.Errorf("accrual engine is nil")
	ErrInvalidDay    = fmt.Errorf("accrual business day format invalid (YYYY-MM-DD)")
	ErrActorRequired = fmt.Errorf("accrual actor required")
	ErrBatchNotFound = fmt.Errorf("accrual batch not found")
	ErrInvalidKind   = fmt.Errorf("accrual line kind invalid")
)
