package forbidgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
)

   func init() {
	  apierr.Register(ErrUserRequired, http.StatusBadRequest)
	  apierr.Register(ErrInvalidActor, http.StatusBadRequest)
	  apierr.Register(ErrForbidGenStale,
		   http.StatusConflict)
	   apierr.Register(ErrBindNotFound, http.StatusNotFound)
	  apierr.Register(ErrBindExpired, http.StatusGone)
	  apierr.Register(ErrBindRevoked, http.StatusConflict)
	   apierr.Register(ErrBindConsumed, http.StatusConflict)
	  apierr.Register(ErrDriftWriteBlock,
		   http.StatusLocked)
	  apierr.Register(ErrProofMissing, http.StatusUnprocessableEntity)
	   apierr.Register(ErrProofConfirmConflict, http.StatusConflict)
	  apierr.Register(ErrNoProofToExport, http.StatusNotFound)
   }

// RegisterWithDeps 从原始依赖装配禁提闸路由（避免与 app 包循环依赖）。
   func RegisterWithDeps(v1 *gin.RouterGroup, kv persist.KV, sqlDB *persist.SQLDB) {
	  if v1 == nil || kv == nil {
		  return
	   }
	   var repo *Repo
	   if sqlDB != nil {
		   repo = NewRepo(sqlDB)
	  }
	   gate := NewGate(kv, repo)
	  drift := NewDriftService(repo)
	  h := NewHandler(gate, drift)
	   h.Register(v1)
  }
