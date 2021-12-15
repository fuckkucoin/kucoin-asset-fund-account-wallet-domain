package usageleasegate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, _ *app.Runtime) {
	   eng := New(Config{Store: NewMemStore(), AutoResv: true})
	   RegisterRoutes(v1, eng)
   }
