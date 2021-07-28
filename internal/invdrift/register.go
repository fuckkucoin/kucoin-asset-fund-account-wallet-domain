package invdrift

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"github.com/gin-gonic/gin"
)

  func init() {
	  apierr.Register(ErrUnavailable,
		  http.StatusServiceUnavailable)
	   apierr.Register(ErrStore, http.StatusServiceUnavailable)
	   apierr.Register(ErrBatchNotFound,
		   http.StatusNotFound)
	  apierr.Register(ErrItemNotFound, http.StatusNotFound)
	   apierr.Register(ErrAdviceNotFound, http.StatusNotFound)
	  apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrBadState, http.StatusConflict)
	   apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrAmount,
		  http.StatusBadRequest)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrAdviceActive, http.StatusConflict)
	   apierr.Register(ErrIntegrity, http.StatusConflict)
	   apierr.Register(ErrConflict, http.StatusConflict)
  }

// Register 挂载 invdrift 路由到 v1 路由组。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var sqlStore Store
	   if rt != nil && rt.SQL != nil {
		   sqlStore = NewSQLStore(rt.SQL.DB)
	  }
	  var custSvc *custody.Service
	  if rt != nil {
		   custSvc = custody.New(rt.Store, rt.Ledger, nil, map[string]string{
			   "USDT": "500000", "BTC": "80", "ETH": "800",
		  })
	  }
	   var invBook *hotinv.Book
	  if rt != nil {
		   invBook = hotinv.FromHotMax(map[string]string{
			  "USDT": "500000", "BTC": "80", "ETH": "800",
		  })
	   }
	   var svc *Service
	  if rt != nil {
		  svc = New(sqlStore, rt.Store, custSvc, invBook,
			  WithLogger(rt.Log),
		   )
	   } else {
		   svc = New(nil, nil, nil, nil)
	   }
	   RegisterRoutes(v1, svc)
  }
