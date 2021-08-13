package ledbatch

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("ledger batch unavailable")
	ErrUser   = fmt.Errorf("ledger batch user invalid")
	ErrAsset  = fmt.Errorf("ledger batch asset invalid")
	ErrAmount = fmt.Errorf("ledger batch amount invalid")
	ErrDir    = fmt.Errorf("ledger batch direction invalid")
	ErrIdem   = fmt.Errorf("ledger batch idempotency conflict")
	ErrBatch  = fmt.Errorf("ledger batch invalid")
	ErrHalt   = fmt.Errorf("ledger batch halted")
	ErrEmpty    = fmt.Errorf("ledger batch empty")
	ErrState    = fmt.Errorf("ledger batch state invalid")
	ErrActor    = fmt.Errorf("ledger batch actor invalid")
	ErrConserve = fmt.Errorf("ledger batch does not conserve")
	ErrItem = fmt.Errorf("ledger batch item invalid")
)
// temporary guard removed after the production check
