package wdpartial

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrBatchNotFound, http.StatusNotFound)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrBatchID, http.StatusBadRequest)
	  apierr.Register(ErrResidueID, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrUserID, http.StatusBadRequest)
	  apierr.Register(ErrTerminal, http.StatusConflict)
	   apierr.Register(ErrNotOpen, http.StatusConflict)
	   apierr.Register(ErrBatchState, http.StatusConflict)
	  apierr.Register(ErrNoLease,
		   http.StatusForbidden)
	   apierr.Register(ErrStaleGen, http.StatusConflict)
	  apierr.Register(ErrLeaseExpired, http.StatusConflict)
	   apierr.Register(ErrLeaseConflict, http.StatusConflict)
	   apierr.Register(ErrLeaseNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrLeaseSelf, http.StatusConflict)
	   apierr.Register(ErrProofDuplicate, http.StatusConflict)
	   apierr.Register(ErrProofMissing,
		   http.StatusUnprocessableEntity)
	   apierr.Register(ErrProofChecksum, http.StatusUnprocessableEntity)
	   apierr.Register(ErrProofStore, http.StatusServiceUnavailable)
	   apierr.Register(ErrConserve, http.StatusConflict)
	  apierr.Register(ErrCloseoutExists, http.StatusConflict)
	   apierr.Register(ErrObserver, http.StatusServiceUnavailable)
	   apierr.Register(ErrSourceMismatch, http.StatusBadRequest)
  }

// Register 将 wdpartial 端点注册到路由组。
// 若 rt 不为 nil，则从 Runtime 注入级 BatchReader 和 OccupyObserver。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := NewService()
	  if rt != nil {
		  // 环境注入：BatchReader 挂接 wdbatch 只读适配器
		  // OccupyObserver 挂接账本/冻结只读查询
		   // （适配器在 wdbatch-residue 层实现，此处留桩供接线）
		   _ = rt
	   }
	   registerHTTP(v1, svc)
   }
