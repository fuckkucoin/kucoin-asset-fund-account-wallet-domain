package fiatrail

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("fiat rail unavailable")
	ErrKind        = fmt.Errorf("fiat rail kind invalid")
	ErrState       = fmt.Errorf("fiat rail illegal state")
	ErrBooking     = fmt.Errorf("fiat booking not found")
	ErrIdem        = fmt.Errorf("fiat booking idempotency conflict")
	ErrKYC         = fmt.Errorf("fiat kyc gate refused")
	ErrPSP         = fmt.Errorf("mock psp refused")
	ErrLimit = fmt.Errorf("fiat rail limit exceeded")
	ErrCard  = fmt.Errorf("fiat card reference invalid")
	ErrLedger = fmt.Errorf("fiat rail ledger refused")
	ErrRefund = fmt.Errorf("fiat refund refused")
	ErrUser   = fmt.Errorf("fiat user invalid")
	ErrAsset = fmt.Errorf("fiat asset invalid")
	ErrAmount = fmt.Errorf("fiat amount invalid")
	ErrActor    = fmt.Errorf("fiat actor invalid")
	ErrReason   = fmt.Errorf("fiat reason invalid")
	ErrCurrency = fmt.Errorf("fiat currency invalid")
)
