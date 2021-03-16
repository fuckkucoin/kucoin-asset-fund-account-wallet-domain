package chainad

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("chain adapter unavailable")
	ErrChain       = fmt.Errorf("unsupported chain family")
	ErrAddress = fmt.Errorf("chain address invalid")
	ErrHash   = fmt.Errorf("tx hash invalid")
	ErrAmount = fmt.Errorf("observed amount invalid")
	ErrNotFound = fmt.Errorf("chain tx not found")
	ErrReorged = fmt.Errorf("chain tx reorged")
	ErrPending  = fmt.Errorf("chain tx pending")
	ErrFailed   = fmt.Errorf("chain tx failed")
	ErrMismatch = fmt.Errorf("observe mismatch")
	ErrNonce    = fmt.Errorf("submit nonce replay")
	ErrDepth    = fmt.Errorf("confirmations below family depth")
	ErrSubmit   = fmt.Errorf("withdraw submit refused")
)
