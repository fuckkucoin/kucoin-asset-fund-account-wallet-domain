package wdtriplegate

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("wdtriplegate unavailable")
	ErrLeaseNotFound = fmt.Errorf("wdtriplegate lease not found")
	ErrLeaseExpired  = fmt.Errorf("wdtriplegate lease expired")
	ErrLeaseBlocked  = fmt.Errorf("wdtriplegate lease blocked by drift")
	ErrLeaseTerminal     = fmt.Errorf("wdtriplegate lease already terminal")
	ErrInvalidTransition = fmt.Errorf("wdtriplegate invalid state transition")
	ErrDriftDetected     = fmt.Errorf("wdtriplegate generation drift detected")
	ErrDriftCaseNotFound = fmt.Errorf("wdtriplegate drift case not found")
	ErrDriftCaseTerminal = fmt.Errorf("wdtriplegate drift case already closed")
	ErrDualConfirm      = fmt.Errorf("wdtriplegate dual confirmation required (confirmed_by != co_confirmed_by)")
	ErrBindConflict     = fmt.Errorf("wdtriplegate bind conflict: movement already bound to another lease")
	ErrBindNotFound     = fmt.Errorf("wdtriplegate bind not found")
	ErrMovementRequired = fmt.Errorf("wdtriplegate movement_id required")
	ErrLeaseIDRequired = fmt.Errorf("wdtriplegate triple_lease_id required")
	ErrInvalidListGen  = fmt.Errorf("wdtriplegate list_gen must be positive")
	ErrInvalidPolicyVer = fmt.Errorf("wdtriplegate policy_ver must be positive")
	ErrInvalidKYTPin = fmt.Errorf("wdtriplegate kyt_pin must be allow")
	ErrTicketOpen    = fmt.Errorf("wdtriplegate cannot issue lease: open exception ticket exists")
	ErrBroadcastBlocked = fmt.Errorf("wdtriplegate broadcast blocked: lease not bound or drift detected")
)
