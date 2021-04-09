package destentrygate

import "fmt"

var (
	ErrUnavailable   = fmt.Errorf("destentrygate: engine unavailable")
	ErrNoLease       = fmt.Errorf("destentrygate: withdraw requires unexpired dest_entry_lease")
	ErrLeaseNotFound = fmt.Errorf("destentrygate: dest_entry_lease not found")
	ErrLeaseExpired = fmt.Errorf("destentrygate: dest_entry_lease expired")
	ErrLeaseRevoked = fmt.Errorf("destentrygate: dest_entry_lease revoked")
	ErrLeaseConsumed     = fmt.Errorf("destentrygate: dest_entry_lease already consumed")
	ErrLeaseNotIssued    = fmt.Errorf("destentrygate: dest_entry_lease not in issued state")
	ErrLeaseNotBound     = fmt.Errorf("destentrygate: dest_entry_lease not bound")
	ErrInvalidTransition = fmt.Errorf("destentrygate: invalid FSM transition")
	ErrInvalidReq = fmt.Errorf("destentrygate: invalid request")
	ErrIdem = fmt.Errorf("destentrygate: idempotency key required")
	ErrWhitelistOnly  = fmt.Errorf("destentrygate: whitelist-only requires dest_entry_lease")
	ErrPrecheckDenied = fmt.Errorf("destentrygate: destallow precheck denied")
	ErrEntryMissing   = fmt.Errorf("destentrygate: whitelist entry missing")
	ErrEntryExpired   = fmt.Errorf("destentrygate: whitelist temp entry expired")
	ErrEntryRevoked   = fmt.Errorf("destentrygate: whitelist entry revoked")
	ErrBreakglassReclaimed = fmt.Errorf("destentrygate: breakglass reclaimed")
	ErrCooldownNotFull     = fmt.Errorf("destentrygate: restore cooldown not full")
	ErrEntryGenDrift       = fmt.Errorf("destentrygate: entry_gen drift")
	ErrModeDrift           = fmt.Errorf("destentrygate: entry mode drift")
	ErrAddrDrift = fmt.Errorf("destentrygate: user/chain/address drift")
	ErrProofNotFound = fmt.Errorf("destentrygate: dest_entry_drift_proof not found")
	ErrGenNotFound   = fmt.Errorf("destentrygate: entry generation not found")
	ErrWithdrawBlocked = fmt.Errorf("destentrygate: withdraw blocked by fail-closed entry lease")
	ErrStore           = fmt.Errorf("destentrygate: store error")
	ErrRenewDenied     = fmt.Errorf("destentrygate: lease renew denied")
)

// WrapStore 包装仓储错误。
   func WrapStore(op string, err error) error {
	  return fmt.Errorf("%w: %s: %w", ErrStore, op, err)
   }
