package auditchain

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrSegment = fmt.Errorf("audit segment invalid")
	ErrSealed  = fmt.Errorf("audit segment sealed")
	ErrFrozen  = fmt.Errorf("audit segment frozen")
	ErrExport = fmt.Errorf("audit export denied")
	ErrCursor = fmt.Errorf("audit cursor regress")
	ErrDrift  = fmt.Errorf("audit checkpoint drift")
	ErrBroken = fmt.Errorf("audit chain broken")
	ErrLag     = fmt.Errorf("audit ingest lag")
	ErrReplay  = fmt.Errorf("audit replay failed")
	ErrPart    = fmt.Errorf("audit partition invalid")
	ErrOrdinal = fmt.Errorf("audit segment gap")
)

  func init() {
	   apierr.Register(ErrSegment, http.StatusConflict)
	  apierr.Register(ErrSealed, http.StatusConflict)
	  apierr.Register(ErrFrozen, http.StatusConflict)
	  apierr.Register(ErrExport, http.StatusForbidden)
	  apierr.Register(ErrCursor, http.StatusConflict)
	  apierr.Register(ErrDrift, http.StatusConflict)
	  apierr.Register(ErrBroken, http.StatusConflict)
	  apierr.Register(ErrLag, http.StatusConflict)
	   apierr.Register(ErrReplay, http.StatusConflict)
	   apierr.Register(ErrPart, http.StatusBadRequest)
	   apierr.Register(ErrOrdinal, http.StatusConflict)
	   apierr.Register(ErrKind,
		  http.StatusBadRequest)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrBody, http.StatusBadRequest)
   }
