package disputepaygate

import "fmt"

var (
	ErrUnavailable       = fmt.Errorf("disputepaygate: engine unavailable")
	ErrInvalidInput      = fmt.Errorf("disputepaygate: invalid input")
	ErrCaseNotFound      = fmt.Errorf("disputepaygate: case not found")
	ErrLeaseNotFound     = fmt.Errorf("disputepaygate: lease not found")
	ErrLeaseExpired      = fmt.Errorf("disputepaygate: lease expired")
	ErrLeaseRevoked      = fmt.Errorf("disputepaygate: lease revoked")
	ErrLeaseConsumed     = fmt.Errorf("disputepaygate: lease already consumed")
	ErrIllegalTransition = fmt.Errorf("disputepaygate: illegal state transition")
	ErrNoLeaseForCredit  = fmt.Errorf("disputepaygate: ledger credit requires dispute_pay_lease_id")
	ErrNoLeaseForDebit = fmt.Errorf("disputepaygate: ledger debit requires claw_lease")
	ErrEvidenceDrift = fmt.Errorf("disputepaygate: evidence root drift")
	ErrGenMismatch   = fmt.Errorf("disputepaygate: case generation mismatch")
	ErrDoubleResolve = fmt.Errorf("disputepaygate: case already resolved with lease")
	ErrIdemReplayPay = fmt.Errorf("disputepaygate: idempotency replay blocked second payout")
	ErrFourEye = fmt.Errorf("disputepaygate: four-eye required")
	ErrProofNotFound    = fmt.Errorf("disputepaygate: pay proof not found")
	ErrAmountMismatch   = fmt.Errorf("disputepaygate: lease amount mismatch")
	ErrAssetMismatch    = fmt.Errorf("disputepaygate: lease asset mismatch")
	ErrDecisionMismatch = fmt.Errorf("disputepaygate: lease decision mismatch")
	ErrDesk             = fmt.Errorf("disputepaygate: dispute desk reject")
)
