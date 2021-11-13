package shardholdgen

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("shardholdgen: engine unavailable")
	ErrInvalidReq  = fmt.Errorf("shardholdgen: invalid request")
	ErrIdem        = fmt.Errorf("shardholdgen: idempotency key required")
	ErrGenNotFound = fmt.Errorf("shardholdgen: leg_hold_gen not found")
	ErrNoGeneration = fmt.Errorf("shardholdgen: generation required fail-closed")
	ErrGenExpired   = fmt.Errorf("shardholdgen: leg_hold_gen expired")
	ErrGenReleased  = fmt.Errorf("shardholdgen: leg_hold_gen already released")
	ErrInvalidTransition = fmt.Errorf("shardholdgen: invalid FSM transition")
	ErrMutexHeld         = fmt.Errorf("shardholdgen: advance/release mutex held")
	ErrBroadcastLive = fmt.Errorf("shardholdgen: broadcasting leg forbids release")
	ErrOverRelease   = fmt.Errorf("shardholdgen: release exceeds hold")
	ErrDoubleHold  = fmt.Errorf("shardholdgen: double hold rejected")
	ErrMissRelease = fmt.Errorf("shardholdgen: miss-release detected")
	ErrHalfNotFound  = fmt.Errorf("shardholdgen: half-leg case not found")
	ErrHalfLocked    = fmt.Errorf("shardholdgen: half-leg resume mutex held")
	ErrProofNotFound = fmt.Errorf("shardholdgen: shard hold proof not found")
	ErrStore         = fmt.Errorf("shardholdgen: store error")
	ErrResumeDenied  = fmt.Errorf("shardholdgen: half resume denied")
	ErrReleaseDenied = fmt.Errorf("shardholdgen: release assert denied")
	ErrAdvanceDenied = fmt.Errorf("shardholdgen: advance with gen denied")
	ErrLiveHash      = fmt.Errorf("shardholdgen: live-hash write-fence mismatch")
	ErrConflict      = fmt.Errorf("shardholdgen: concurrent advance/release conflict")
)

  func WrapStore(op string, err error) error {
	   if err == nil {
		   return nil
	  }
	  return fmt.Errorf("%w: %s: %w", ErrStore, op, err)
  }
