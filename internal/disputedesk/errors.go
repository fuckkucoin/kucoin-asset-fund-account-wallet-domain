package disputedesk

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("dispute desk unavailable")
	ErrCase        = fmt.Errorf("dispute case not found")
	ErrState       = fmt.Errorf("dispute illegal state")
	ErrChannel = fmt.Errorf("dispute channel invalid")
	ErrUser = fmt.Errorf("dispute user invalid")
	ErrAsset = fmt.Errorf("dispute asset invalid")
	ErrAmount = fmt.Errorf("dispute amount invalid")
	ErrActor    = fmt.Errorf("dispute actor invalid")
	ErrIdem     = fmt.Errorf("dispute idempotency conflict")
	ErrEvidence = fmt.Errorf("dispute evidence invalid")
	ErrHash     = fmt.Errorf("dispute evidence hash mismatch")
	ErrFourEye = fmt.Errorf("dispute four-eye required")
	ErrReason  = fmt.Errorf("dispute reason invalid")
	// ErrFundGateBlocking 资金收口闸阻断：追偿腿尚未完成，禁止结案。
	ErrFundGateBlocking = fmt.Errorf("dispute fund gate blocking: recovery legs incomplete")
)
