package balproof

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("balance proof unavailable")
	ErrUser  = fmt.Errorf("balance proof user invalid")
	ErrAsset = fmt.Errorf("balance proof asset invalid")
	ErrBook  = fmt.Errorf("balance proof book invalid")
	ErrNonce = fmt.Errorf("balance proof nonce invalid")
	ErrReplay   = fmt.Errorf("balance proof nonce replay")
	ErrSig      = fmt.Errorf("balance proof signature mismatch")
	ErrSecret   = fmt.Errorf("balance proof secret missing")
	ErrAmount   = fmt.Errorf("balance proof amount invalid")
	ErrNotFound = fmt.Errorf("balance proof not found")
	ErrExpired  = fmt.Errorf("balance proof nonce expired")
)
