package wdbatch

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("withdraw batch unavailable")
	ErrUser  = fmt.Errorf("withdraw batch user invalid")
	ErrAsset = fmt.Errorf("withdraw batch asset invalid")
	ErrAmount = fmt.Errorf("withdraw batch amount invalid")
	ErrChain = fmt.Errorf("withdraw batch chain invalid")
	ErrAddr  = fmt.Errorf("withdraw batch address invalid")
	ErrIdem  = fmt.Errorf("withdraw batch idempotency conflict")
	ErrBatch = fmt.Errorf("withdraw batch invalid")
	ErrHalt  = fmt.Errorf("withdraw batch halted")
	ErrEmpty = fmt.Errorf("withdraw batch empty")
	ErrState    = fmt.Errorf("withdraw batch state invalid")
	ErrActor    = fmt.Errorf("withdraw batch actor invalid")
	ErrConserve = fmt.Errorf("withdraw batch does not conserve")
	ErrItem     = fmt.Errorf("withdraw batch item invalid")
)
