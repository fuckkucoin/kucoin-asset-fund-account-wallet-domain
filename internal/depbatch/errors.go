package depbatch

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("deposit batch unavailable")
	ErrUser  = fmt.Errorf("deposit batch user invalid")
	ErrAsset = fmt.Errorf("deposit batch asset invalid")
	ErrChain = fmt.Errorf("deposit batch chain invalid")
	ErrHash  = fmt.Errorf("deposit batch tx hash invalid")
	ErrIdem  = fmt.Errorf("deposit batch idempotency conflict")
	ErrBatch = fmt.Errorf("deposit batch invalid")
	ErrHalt = fmt.Errorf("deposit batch halted")
	ErrEmpty = fmt.Errorf("deposit batch empty")
	ErrState = fmt.Errorf("deposit batch state invalid")
	ErrActor = fmt.Errorf("deposit batch actor invalid")
	ErrDup   = fmt.Errorf("deposit batch duplicate hash")
	ErrItem = fmt.Errorf("deposit batch item invalid")
)
