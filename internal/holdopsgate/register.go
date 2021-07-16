package holdopsgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUserInvalid, http.StatusBadRequest)
	   apierr.Register(ErrAssetInvalid, http.StatusBadRequest)
	  apierr.Register(ErrAmountInvalid, http.StatusBadRequest)
	  apierr.Register(ErrActorInvalid,
		  http.StatusBadRequest)
	   apierr.Register(ErrNonceInvalid, http.StatusBadRequest)
	   apierr.Register(ErrKindInvalid, http.StatusBadRequest)
	  apierr.Register(ErrTokenInvalid, http.StatusBadRequest)
	  apierr.Register(ErrGapInvalid,
		   http.StatusBadRequest)
	  apierr.Register(ErrTicketInvalid, http.StatusBadRequest)
	   apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseMissing, http.StatusForbidden)
	   apierr.Register(ErrLeaseExpired,
		   http.StatusConflict)
	  apierr.Register(ErrLeaseRevoked, http.StatusConflict)
	  apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	   apierr.Register(ErrLeaseTerminal, http.StatusConflict)
	  apierr.Register(ErrLeaseNotOpen,
		  http.StatusConflict)
	  apierr.Register(ErrLeaseQuota, http.StatusConflict)
	  apierr.Register(ErrLeaseUser, http.StatusConflict)
	  apierr.Register(ErrLeaseAmount, http.StatusConflict)
	   apierr.Register(ErrLeaseAsset,
		  http.StatusConflict)
	   apierr.Register(ErrLeaseKind, http.StatusConflict)
	   apierr.Register(ErrLeaseToken, http.StatusConflict)
	  apierr.Register(ErrLeaseGap, http.StatusConflict)
	   apierr.Register(ErrStaleGen, http.StatusConflict)
	   apierr.Register(ErrFenceOn,
		  http.StatusConflict)
	   apierr.Register(ErrConcurrent, http.StatusConflict)
	  apierr.Register(ErrDualRelease,
		  http.StatusConflict)
	   apierr.Register(ErrReleaseClose, http.StatusConflict)
	   apierr.Register(ErrGapOpenToken, http.StatusConflict)
	   apierr.Register(ErrAckNotRelease, http.StatusConflict)
	   apierr.Register(ErrTokenClosed, http.StatusConflict)
	  apierr.Register(ErrTokenGone, http.StatusNotFound)
	  apierr.Register(ErrGapClosed,
		  http.StatusConflict)
	   apierr.Register(ErrGapGone, http.StatusNotFound)
	   apierr.Register(ErrProofBlocked, http.StatusConflict)
	   apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrProofClosed, http.StatusConflict)
	   apierr.Register(ErrFourEyeSame, http.StatusForbidden)
	   apierr.Register(ErrOperatorInvalid, http.StatusBadRequest)
	   apierr.Register(ErrReasonRequired, http.StatusBadRequest)
	  apierr.Register(ErrIsolation, http.StatusBadRequest)
	   apierr.Register(ErrIdempotentNonce, http.StatusConflict)
	   apierr.Register(ErrReplayAudit, http.StatusConflict)
  }
