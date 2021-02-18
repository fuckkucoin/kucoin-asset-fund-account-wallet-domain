package backpress

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New(64, 4)
	  _ = rt
	  v1.POST("/backpress/offer", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Kind string `json:"kind"`
			   Payload string `json:"payload"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  j, err := svc.Offer(body.Kind, body.Payload)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, j)
	   })
	  v1.POST("/backpress/run", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, svc.RunOnce(16))
	  })
	   v1.GET("/backpress/stats", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, svc.Stats())
	   })
  }
