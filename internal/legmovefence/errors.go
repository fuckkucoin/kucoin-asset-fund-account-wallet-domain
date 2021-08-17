package legmovefence

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("legmovefence: engine unavailable")
	ErrInvalidReq    = fmt.Errorf("legmovefence: invalid request")
	ErrIdem          = fmt.Errorf("legmovefence: idempotency key required")
	ErrFenceNotFound = fmt.Errorf("legmovefence: leg_move_fence not found")
	ErrFenceExpired = fmt.Errorf("legmovefence: leg_move_fence expired")
	ErrFenceRevoked      = fmt.Errorf("legmovefence: leg_move_fence revoked")
	ErrFenceMoved        = fmt.Errorf("legmovefence: inventory already moved for fence")
	ErrFenceConsumed     = fmt.Errorf("legmovefence: fence already consumed")
	ErrNoFence           = fmt.Errorf("legmovefence: transfer requires issued fence")
	ErrInvalidTransition = fmt.Errorf("legmovefence: invalid FSM transition")
	ErrFingerprint       = fmt.Errorf("legmovefence: amount/path fingerprint mismatch")
	ErrGeneration        = fmt.Errorf("legmovefence: fence generation stale after halt")
	ErrRegionHalted      = fmt.Errorf("legmovefence: source or destination region halted")
	ErrHalfNotFound  = fmt.Errorf("legmovefence: half-move case not found")
	ErrHalfLocked    = fmt.Errorf("legmovefence: half-move resume mutex held")
	ErrProofNotFound = fmt.Errorf("legmovefence: geo double-move proof not found")
	ErrDoubleMove = fmt.Errorf("legmovefence: second transfer rejected")
	ErrStore      = fmt.Errorf("legmovefence: store error")
	ErrRetryDenied = fmt.Errorf("legmovefence: retry assert fence denied")
	ErrResumeDenied = fmt.Errorf("legmovefence: ops resume denied")
)

  func WrapStore(op string, err error) error {
	  return fmt.Errorf("%w: %s: %w", ErrStore, op, err)
   }
// temporary guard removed after the production check
