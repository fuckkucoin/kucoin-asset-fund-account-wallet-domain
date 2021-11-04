package scoreleasegate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUserInvalid, http.StatusBadRequest)
	  apierr.Register(ErrAssetInvalid,
		   http.StatusBadRequest)
	   apierr.Register(ErrAmountInvalid,
		  http.StatusBadRequest)
	   apierr.Register(ErrDestInvalid, http.StatusBadRequest)
	  apierr.Register(ErrChainInvalid, http.StatusBadRequest)
	  apierr.Register(ErrNonceInvalid,
		  http.StatusBadRequest)
	  apierr.Register(ErrActorInvalid, http.StatusBadRequest)
	  apierr.Register(ErrTicketInvalid, http.StatusBadRequest)
	   apierr.Register(ErrKindInvalid, http.StatusBadRequest)
	   apierr.Register(ErrPackInvalid, http.StatusBadRequest)
	  apierr.Register(ErrModeInvalid, http.StatusBadRequest)
	  apierr.Register(ErrEvalDenied, http.StatusForbidden)
	  apierr.Register(ErrEvalHalt, http.StatusConflict)
	   apierr.Register(ErrEvalFailed, http.StatusServiceUnavailable)
	  apierr.Register(ErrShadowLease, http.StatusForbidden)
	  apierr.Register(ErrHoldReview, http.StatusForbidden)
	   apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseMissing, http.StatusForbidden)
	  apierr.Register(ErrLeaseExpired,
		  http.StatusConflict)
	  apierr.Register(ErrLeaseRevoked, http.StatusConflict)
	  apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	  apierr.Register(ErrLeaseTerminal,
		   http.StatusConflict)
	  apierr.Register(ErrLeaseNotOpen, http.StatusConflict)
	   apierr.Register(ErrLeaseQuota, http.StatusConflict)
	   apierr.Register(ErrLeaseUser, http.StatusConflict)
	   apierr.Register(ErrLeaseAmount, http.StatusConflict)
	   apierr.Register(ErrLeaseDest, http.StatusConflict)
	   apierr.Register(ErrLeaseAsset,
		  http.StatusConflict)
	   apierr.Register(ErrLeaseChain, http.StatusConflict)
	  apierr.Register(ErrHaltFlip, http.StatusConflict)
	  apierr.Register(ErrPackFlip, http.StatusConflict)
	  apierr.Register(ErrModeFlip,
		   http.StatusConflict)
	  apierr.Register(ErrWindowDrift, http.StatusConflict)
	  apierr.Register(ErrFingerprintDrift, http.StatusConflict)
	  apierr.Register(ErrStaleGen,
		   http.StatusConflict)
	   apierr.Register(ErrConcurrent, http.StatusConflict)
	   apierr.Register(ErrProofBlocked, http.StatusConflict)
	   apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrProofClosed, http.StatusConflict)
	   apierr.Register(ErrFourEyeSame, http.StatusForbidden)
	  apierr.Register(ErrOperatorInvalid, http.StatusBadRequest)
	  apierr.Register(ErrReasonRequired, http.StatusBadRequest)
	  apierr.Register(ErrIsolationViolation, http.StatusBadRequest)
	   apierr.Register(ErrIdempotentNonce, http.StatusConflict)
   }
