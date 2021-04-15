package cvquotelease

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, _ *app.Runtime) {
	   marks := NewMemoryMarks()
	   marks.Put("BTC", "USDT", "42000")
	  eng := New(Config{
		   Store:  NewMemStore(),
		   Marks:  marks,
		   Strict: true,
	   })
	  RegisterRoutes(v1, eng)
   }
