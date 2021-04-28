package creditoutgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrBoardKey, http.StatusBadRequest)
	   apierr.Register(ErrCreditKey,
		   http.StatusBadRequest)
	  apierr.Register(ErrBoardClosed, http.StatusForbidden)
	  apierr.Register(ErrGenDrift, http.StatusConflict)
	   apierr.Register(ErrLeaseRevoked, http.StatusForbidden)
	  apierr.Register(ErrLeaseNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrAmountConflict, http.StatusConflict)
	  apierr.Register(ErrDualCredit, http.StatusConflict)
	  apierr.Register(ErrEpochToken,
		   http.StatusConflict)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrOperator, http.StatusBadRequest)
	  apierr.Register(ErrAlreadyDelivered, http.StatusConflict)
  }
// temporary guard removed after the production check
