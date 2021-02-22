package custody

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func registerLots(v1 *gin.RouterGroup, led *Ledger) {
	  if led == nil {
		  led = NewLedger()
	  }
	  v1.GET("/custody/lots", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{
			  "lots": led.Lots(c.Query("side"), c.Query("asset")),
			  "host": "custody-boundary-lab.kucoin.net",
		   })
	  })
	  v1.POST("/custody/lots/receive", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Side  string `json:"side"`
			   Asset string `json:"asset"`
			   Amount string `json:"amount"`
			  Kind   string `json:"kind"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if _, err := validate.Asset(body.Asset); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   lot, mv, err := led.Receive(body.Side, body.Asset, body.Amount, body.Kind)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"lot": lot, "movement": mv})
	   })
	   v1.GET("/custody/lots/movements", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"movements": led.Movements(validate.Limit(0, 50, 200))})
	  })
   }
