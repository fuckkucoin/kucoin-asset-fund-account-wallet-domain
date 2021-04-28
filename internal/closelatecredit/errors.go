package closelatecredit

import "errors"

var (
	ErrUnavailable = errors.New("closelatecredit: engine unavailable")
	ErrNotFound       = errors.New("closelatecredit: not found")
	ErrDuplicate      = errors.New("closelatecredit: duplicate")
	ErrInvalidTransit = errors.New("closelatecredit: invalid epoch or late-credit transit")
	ErrMissingEpoch = errors.New("closelatecredit: close_epoch required")
	ErrEpochMismatch = errors.New("closelatecredit: close_epoch mismatch")
	ErrEpochStale = errors.New("closelatecredit: close_epoch stale after reopen")
	ErrClosedRefuse  = errors.New("closelatecredit: credit refused after account closed")
	ErrDisputeHold   = errors.New("closelatecredit: credit parked in dispute hold")
	ErrNotClosed     = errors.New("closelatecredit: account is not closed")
	ErrNotReopenable = errors.New("closelatecredit: epoch cannot reopen from current state")
	ErrAlreadyClosed = errors.New("closelatecredit: epoch already closed")
	ErrAlreadyOpen    = errors.New("closelatecredit: epoch already open")
	ErrUserRequired   = errors.New("closelatecredit: user_id required")
	ErrSourceRequired = errors.New("closelatecredit: credit source required")
	ErrAmount = errors.New("closelatecredit: amount rejected")
	ErrAsset  = errors.New("closelatecredit: asset rejected")
	ErrIdemConflict = errors.New("closelatecredit: credit idempotency conflict")
	ErrForceDenied  = errors.New("closelatecredit: force credit to live book denied")
	ErrAccountView  = errors.New("closelatecredit: account status view unavailable")
	ErrHoldNotFound  = errors.New("closelatecredit: dispute hold not found")
	ErrProofNotFound = errors.New("closelatecredit: late credit proof not found")
	ErrResolveDenied = errors.New("closelatecredit: dispute resolve denied")
	ErrDoubleCredit  = errors.New("closelatecredit: live double credit refused")
	ErrExportDenied = errors.New("closelatecredit: proof export denied without digest")
)

// proofError 将证明编号附在错误上。
type proofError struct {
	base error
	proofID string
}

  func (e proofError) Error() string {
	   if e.proofID == "" {
		  return e.base.Error()
	  }
	   return e.base.Error() + ": proof=" + e.proofID
  }

  func (e proofError) Unwrap() error { return e.base }

  func wrapProof(err error, proofID string) error {
	  if err == nil {
		  return nil
	  }
	   return proofError{base: err, proofID: proofID}
   }

// ProofIDOf 提取证明编号。
   func ProofIDOf(err error) string {
	  var pe proofError
	  if errors.As(err, &pe) {
		  return pe.proofID
	  }
	  return ""
  }
