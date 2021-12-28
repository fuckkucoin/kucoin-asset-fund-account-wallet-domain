package wdshard

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("wdshard service unavailable")
	ErrNotFound    = fmt.Errorf("wdshard plan not found")
	ErrLegNotFound = fmt.Errorf("wdshard leg not found")
	ErrPolicyNotFound = fmt.Errorf("wdshard policy not found")
	ErrIdem           = fmt.Errorf("wdshard idempotency conflict")
	ErrState    = fmt.Errorf("wdshard invalid state transition")
	ErrConserve = fmt.Errorf("wdshard amount does not conserve")
	ErrNoPlan   = fmt.Errorf("wdshard plan id required")
	ErrNoUser = fmt.Errorf("wdshard user id required")
	ErrNoAsset = fmt.Errorf("wdshard asset required")
	ErrNoChain = fmt.Errorf("wdshard chain required")
	ErrNoAddress  = fmt.Errorf("wdshard to_address required")
	ErrAmount     = fmt.Errorf("wdshard amount invalid")
	ErrAmountZero = fmt.Errorf("wdshard amount must be positive")
	ErrPolicyInvalid = fmt.Errorf("wdshard policy invalid")
	ErrTooManyLegs = fmt.Errorf("wdshard too many legs")
	ErrLegAmount      = fmt.Errorf("wdshard leg amount below minimum")
	ErrPaused         = fmt.Errorf("wdshard plan paused")
	ErrAlreadyRunning = fmt.Errorf("wdshard plan already running")
	ErrLiveLock       = fmt.Errorf("wdshard live-hash shard cannot be auto-unfrozen")
	ErrActor          = fmt.Errorf("wdshard actor required")
	ErrHalt = fmt.Errorf("wdshard halted")
	ErrInvalid      = fmt.Errorf("wdshard: invalid argument")
	ErrPlanNotFound = fmt.Errorf("wdshard: plan not found")
	ErrHoldNotFound = fmt.Errorf("wdshard: hold not found")
	ErrHoldExceed  = fmt.Errorf("wdshard: release exceeds hold")
	ErrBroadcast   = fmt.Errorf("wdshard: broadcast submit failed")
	ErrAlreadyHeld = fmt.Errorf("wdshard: leg already holds freeze")
	ErrNoHold      = fmt.Errorf("wdshard: leg has no freeze")
)

  func Wrap(op string, err error) error {
	   if err == nil {
		   return nil
	  }
	  return fmt.Errorf("wdshard: %s: %w", op, err)
  }
