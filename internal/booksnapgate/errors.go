package booksnapgate

import "fmt"

var (
	ErrLeaseNotFound = fmt.Errorf("booksnapgate: snap lease not found")
	ErrLeaseExpired  = fmt.Errorf("booksnapgate: snap lease expired")
	ErrLeaseSealed   = fmt.Errorf("booksnapgate: snap lease already sealed")
	ErrLeaseDrifted = fmt.Errorf("booksnapgate: snap lease drifted")
	ErrLeaseAborted = fmt.Errorf("booksnapgate: snap lease aborted")
	ErrLeaseNotAssembling = fmt.Errorf("booksnapgate: snap lease not in assembling state")
	ErrLeaseNotPinned     = fmt.Errorf("booksnapgate: snap lease not in pinned state")
	ErrDriftDetected = fmt.Errorf("booksnapgate: mid-assemble drift detected")
	ErrConserveFailed = fmt.Errorf("booksnapgate: multi-book conserve check failed")
	ErrUserInvalid  = fmt.Errorf("booksnapgate: user id invalid")
	ErrAssetInvalid = fmt.Errorf("booksnapgate: asset invalid")
	ErrBookInvalid   = fmt.Errorf("booksnapgate: book invalid")
	ErrWindowInvalid = fmt.Errorf("booksnapgate: snap window invalid")
	ErrStoreUnavailable = fmt.Errorf("booksnapgate: store unavailable")
	ErrProofNotFound    = fmt.Errorf("booksnapgate: drift proof not found")
	ErrIdempotentReuse = fmt.Errorf("booksnapgate: idempotent reuse of sealed result")
	ErrPinConflict = fmt.Errorf("booksnapgate: pin conflict for same window")
	ErrRePinRequired = fmt.Errorf("booksnapgate: re-pin required after drift")
	ErrAssembleAborted   = fmt.Errorf("booksnapgate: assemble aborted due to drift")
	ErrIllegalTransition = fmt.Errorf("booksnapgate: illegal state transition")
	ErrSnapGenMismatch   = fmt.Errorf("booksnapgate: snap gen mismatch")
	ErrNilEngine         = fmt.Errorf("booksnapgate: engine is nil")
	ErrEmptyBills    = fmt.Errorf("booksnapgate: no bills to assemble")
	ErrTTLInvalid    = fmt.Errorf("booksnapgate: ttl must be positive")
	ErrRepinExceeded = fmt.Errorf("booksnapgate: max repin attempts exceeded")
)

  func IsRetryable(err error) bool {
	  switch err {
	   case ErrDriftDetected, ErrRePinRequired, ErrAssembleAborted:
		  return true
	  default:
		  return false
	  }
   }
