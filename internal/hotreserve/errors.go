package hotreserve

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrUnavailable = errSent("hot reserve unavailable")
	ErrUnread      = errSent("hot reserve inventory unreadable")
	ErrAsset  = errSent("hot reserve asset invalid")
	ErrAmount = errSent("hot reserve amount invalid")
	ErrIdem   = errSent("hot reserve idempotency conflict")
	ErrNotFound  = errSent("hot reserve not found")
	ErrState     = errSent("hot reserve state conflict")
	ErrShortfall = errSent("hot reserve conservative shortfall")
	ErrForecastMissing = errSent("hot reserve forecast missing")
	ErrForecastStale   = errSent("hot reserve forecast stale")
	ErrAlignDrift      = errSent("hot reserve align drift")
	ErrDual = errSent("hot reserve force release requires a distinct approver")
	ErrBroadcastHeld = errSent("hot reserve broadcast held")
	ErrDuplicate = errSent("hot reserve intent already open")
	ErrDegraded  = errSent("hot reserve store degraded")
)

type sentinel string

   func errSent(s string) error { return sentinel(s) }

   func (e sentinel) Error() string { return string(e) }

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUnread, http.StatusServiceUnavailable)
	   apierr.Register(ErrDegraded, http.StatusServiceUnavailable)
	   apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrIdem,
		  http.StatusConflict)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrState,
		  http.StatusConflict)
	  apierr.Register(ErrShortfall, http.StatusConflict)
	  apierr.Register(ErrForecastMissing, http.StatusConflict)
	   apierr.Register(ErrForecastStale, http.StatusConflict)
	   apierr.Register(ErrAlignDrift, http.StatusConflict)
	  apierr.Register(ErrDual,
		   http.StatusForbidden)
	  apierr.Register(ErrBroadcastHeld, http.StatusConflict)
	  apierr.Register(ErrDuplicate, http.StatusConflict)
  }
