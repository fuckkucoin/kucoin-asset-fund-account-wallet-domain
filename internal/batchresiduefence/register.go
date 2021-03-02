package batchresiduefence

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

  func init() {
	  apierr.Register(ErrNilEngine, http.StatusServiceUnavailable)
	  apierr.Register(ErrStoreUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrBatchID, http.StatusBadRequest)
	   apierr.Register(ErrItemIdem, http.StatusBadRequest)
	   apierr.Register(ErrActor,
		   http.StatusBadRequest)
	  apierr.Register(ErrFence, http.StatusNotFound)
	   apierr.Register(ErrFenceState, http.StatusConflict)
	   apierr.Register(ErrFenceExpired, http.StatusConflict)
	  apierr.Register(ErrFenceReleased, http.StatusConflict)
	   apierr.Register(ErrGeneration, http.StatusConflict)
	   apierr.Register(ErrFingerprint, http.StatusConflict)
	  apierr.Register(ErrDupFreeze, http.StatusConflict)
	  apierr.Register(ErrItemSubmitted, http.StatusConflict)
	  apierr.Register(ErrItemSkipped, http.StatusConflict)
	   apierr.Register(ErrItemNotFailed, http.StatusConflict)
	  apierr.Register(ErrMutexHeld, http.StatusConflict)
	   apierr.Register(ErrHalfOpen, http.StatusConflict)
	   apierr.Register(ErrSecondFullFreeze,
		  http.StatusConflict)
	   apierr.Register(ErrFailClosed, http.StatusForbidden)
	  apierr.Register(ErrIllegalTransition, http.StatusConflict)
	  apierr.Register(ErrHalfIllegal, http.StatusConflict)
	  apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrUser,
		  http.StatusBadRequest)
	   apierr.Register(ErrViewer, http.StatusFailedDependency)
	   apierr.Register(ErrMaxGeneration, http.StatusConflict)
	  apierr.Register(ErrProof, http.StatusBadRequest)
	   apierr.Register(ErrHalf,
		  http.StatusBadRequest)
	  apierr.Register(ErrSlot, http.StatusConflict)
   }

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  _ = rt
	  eng := New(Config{})
	   RegisterRoutes(v1, eng)
  }
// temporary guard removed after the production check
