package occseggate

import "errors"

var (
	ErrUnavailable = errors.New("occseggate: engine unavailable")
	ErrNotFound    = errors.New("occseggate: not found")
	ErrDuplicate   = errors.New("occseggate: duplicate")
	ErrInvalidTransit = errors.New("occseggate: invalid segment or proof transit")
	ErrMissingGen     = errors.New("occseggate: occ_seg_gen required")
	ErrStaleGen  = errors.New("occseggate: occ_seg_gen stale")
	ErrMutexHeld = errors.New("occseggate: commit/abort mutex held")
	ErrUnexpectedSeg = errors.New("occseggate: unexpected occupancy segment state")
	ErrAlreadyFrozen = errors.New("occseggate: segment frozen; abort must not succeed")
	ErrAlreadyAborted = errors.New("occseggate: segment aborted; commit must not succeed")
	ErrRevoked    = errors.New("occseggate: occupancy generation revoked by expire")
	ErrEpochStale = errors.New("occseggate: ticket epoch stale after expire reopen")
	ErrIdemConflict    = errors.New("occseggate: occupancy idempotency conflict")
	ErrLiveUnfreeze    = errors.New("occseggate: live-chain unfreeze refused")
	ErrGhostPair       = errors.New("occseggate: ghost occupancy pair proven")
	ErrDeskUnavailable = errors.New("occseggate: wdrelease desk unavailable")
	ErrForceDenied     = errors.New("occseggate: force ack denied")
	ErrAmount = errors.New("occseggate: amount rejected")
	ErrAsset  = errors.New("occseggate: asset rejected")
)

// wrapProof 将证明 ID 附在错误上（供调用方导出）。

type proofError struct {
	base    error
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
