package fundpref

import (
	"errors"
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrUser = fmt.Errorf("fund preference user invalid")
	ErrAccount  = fmt.Errorf("fund preference default account invalid")
	ErrCurrency = fmt.Errorf("fund preference display currency invalid")
	ErrOrder = fmt.Errorf("fund preference asset order invalid")
	ErrMarket = fmt.Errorf("fund preference rejects market fields")
	ErrCorrupt = fmt.Errorf("fund preference payload corrupt")
	ErrStore = fmt.Errorf("fund preference store unavailable")
	ErrActor = fmt.Errorf("fund preference actor required")
	ErrInvalidInput = errors.New("fundpref: invalid input")
	ErrInflightCredit     = errors.New("fundpref: inflight credit exists, switch rejected; use pending queue")
	ErrGenerationMismatch = errors.New("fundpref: pref generation mismatch")
	ErrLeaseExpired       = errors.New("fundpref: pref lease expired")
	ErrLeaseInvalid       = errors.New("fundpref: pref lease invalid state")
	ErrDriftUnproven = errors.New("fundpref: pref drift case unproven")
	ErrNotFound = errors.New("fundpref: not found")
	ErrStaleFence = errors.New("fundpref: stale fence")
)

  func init() {
	  apierr.Register(ErrUser,
		  http.StatusBadRequest)
	   apierr.Register(ErrAccount, http.StatusBadRequest)
	   apierr.Register(ErrCurrency, http.StatusBadRequest)
	  apierr.Register(ErrOrder,
		  http.StatusBadRequest)
	  apierr.Register(ErrMarket, http.StatusBadRequest)
	  apierr.Register(ErrCorrupt, http.StatusConflict)
	   apierr.Register(ErrStore, http.StatusServiceUnavailable)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrInvalidInput, http.StatusBadRequest)
	   apierr.Register(ErrInflightCredit,
		  http.StatusConflict)
	  apierr.Register(ErrGenerationMismatch, http.StatusConflict)
	  apierr.Register(ErrLeaseExpired, http.StatusConflict)
	  apierr.Register(ErrLeaseInvalid, http.StatusConflict)
	  apierr.Register(ErrDriftUnproven, http.StatusConflict)
	  apierr.Register(ErrNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrStaleFence, http.StatusConflict)
   }
