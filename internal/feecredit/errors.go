package feecredit

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("fee credit unavailable")
	ErrUser  = fmt.Errorf("fee credit user invalid")
	ErrAsset = fmt.Errorf("fee credit asset invalid")
	ErrAmount = fmt.Errorf("fee credit amount invalid")
	ErrIdem  = fmt.Errorf("fee credit idempotency conflict")
	ErrKind  = fmt.Errorf("fee credit kind invalid")
	ErrQuote = fmt.Errorf("fee credit quote refused")
	ErrJournal = fmt.Errorf("fee credit journal unbalanced")
	ErrCap     = fmt.Errorf("fee credit daily cap")
	ErrFence = fmt.Errorf("fee credit fence stale")
)
