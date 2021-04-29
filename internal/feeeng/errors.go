package feeeng

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("fee engine unavailable")
	ErrUnknownKind  = fmt.Errorf("unknown fee kind")
	ErrUnknownAsset = fmt.Errorf("unknown fee asset")
	ErrSchedule = fmt.Errorf("fee schedule invalid")
	ErrNotFound = fmt.Errorf("fee schedule not found")
	ErrQuote    = fmt.Errorf("fee quote rejected")
)
