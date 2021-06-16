package fundpath

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("fund path unavailable")
	ErrUser        = fmt.Errorf("fund path user invalid")
	ErrAsset       = fmt.Errorf("fund path asset invalid")
	ErrAmount = fmt.Errorf("fund path amount invalid")
	ErrChain  = fmt.Errorf("fund path chain invalid")
	ErrIdem   = fmt.Errorf("fund path idempotency conflict")
	ErrStep     = fmt.Errorf("fund path step refused")
	ErrAlloc    = fmt.Errorf("fund path address alloc refused")
	ErrAttrib   = fmt.Errorf("fund path attribution refused")
	ErrCredit   = fmt.Errorf("fund path credit refused")
	ErrHop      = fmt.Errorf("fund path hop refused")
	ErrAllow    = fmt.Errorf("fund path destination not allowlisted")
	ErrKYT      = fmt.Errorf("fund path kyt refused")
	ErrTravel   = fmt.Errorf("fund path travel envelope refused")
	ErrWithdraw = fmt.Errorf("fund path withdraw preflight refused")
	ErrHot = fmt.Errorf("fund path hot inventory refused")
	ErrFence = fmt.Errorf("fund path fence stale")
	ErrHalt  = fmt.Errorf("fund path halted")
	ErrDust = fmt.Errorf("fund path dust")
	ErrAddr = fmt.Errorf("fund path address invalid")
)
