package loanbook

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("loan book unavailable")
	ErrUser        = fmt.Errorf("loan user invalid")
	ErrAsset       = fmt.Errorf("loan asset invalid")
	ErrAmount = fmt.Errorf("loan amount invalid")
	ErrTerm   = fmt.Errorf("loan term invalid")
	ErrRate   = fmt.Errorf("loan rate invalid")
	ErrLoan   = fmt.Errorf("loan not found")
	ErrState = fmt.Errorf("loan illegal state")
	ErrInstallment = fmt.Errorf("loan installment invalid")
	ErrOverdue     = fmt.Errorf("loan overdue freeze")
	ErrTicket      = fmt.Errorf("loan ticket not found")
	ErrActor    = fmt.Errorf("loan actor invalid")
	ErrIdem     = fmt.Errorf("loan idempotency conflict")
	ErrSchedule = fmt.Errorf("loan schedule invalid")
	ErrCollateral = fmt.Errorf("loan collateral invalid")
	ErrPay        = fmt.Errorf("loan payment refused")
)
