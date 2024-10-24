package wdbatch

import (
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func registerFence(v1 *gin.RouterGroup, svc *Service) {
	   if svc != nil {
		   svc.ensureFence()
	  }
	  batchresiduefence.RegisterRoutes(v1, svc.ensureFence())
	  _ = perm.ActQueryRead
	  _ = mw.BindJSON
   }

   func attachFenceOnRegister(v1 *gin.RouterGroup, svc *Service) {
	   registerFence(v1, svc)
   }
