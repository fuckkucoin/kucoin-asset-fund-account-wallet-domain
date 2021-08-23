package leddiff

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("ledger diff unavailable")
	ErrUser = fmt.Errorf("ledger diff user invalid")
	ErrAsset  = fmt.Errorf("ledger diff asset invalid")
	ErrBook   = fmt.Errorf("ledger diff book invalid")
	ErrAmount = fmt.Errorf("ledger diff amount invalid")
	ErrSnap      = fmt.Errorf("ledger diff snapshot invalid")
	ErrImbalance = fmt.Errorf("ledger diff books do not conserve")
	ErrIdem     = fmt.Errorf("ledger diff idempotency conflict")
	ErrNotFound = fmt.Errorf("ledger diff not found")
	ErrActor = fmt.Errorf("ledger diff actor invalid")
)
