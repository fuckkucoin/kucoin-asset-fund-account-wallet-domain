package vipfee

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("vip fee book unavailable")
	ErrUser = fmt.Errorf("vip fee user invalid")
	ErrAsset = fmt.Errorf("vip fee asset invalid")
	ErrAmount = fmt.Errorf("vip fee amount invalid")
	ErrTier = fmt.Errorf("vip fee tier invalid")
	ErrKind = fmt.Errorf("vip fee kind invalid")
	ErrBPS      = fmt.Errorf("vip fee bps invalid")
	ErrSchedule = fmt.Errorf("vip fee schedule invalid")
	ErrNotFound = fmt.Errorf("vip fee schedule not found")
	ErrActor    = fmt.Errorf("vip fee actor invalid")
	ErrIdem     = fmt.Errorf("vip fee idempotency conflict")
	ErrConserve = fmt.Errorf("vip fee quote does not conserve")
	ErrPin = fmt.Errorf("vip fee pin invalid")
)
