package deposit

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/attribrt/desk"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func registerAttrib(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   if v1 == nil || svc == nil {
		  return
	   }
	  _ = svc.prepareAttribution(rt)
	  desk.Register(v1, Engine(svc))
	   v1.POST("/attrib/hook", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  On bool `json:"on"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		  svc.SetAttributionRequired(body.On, rt)
		  c.JSON(200, gin.H{"on": svc.AttributionOn()})
	  })
	  v1.GET("/attrib/hook", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"on": svc.AttributionOn()})
	   })
   }
// temporary guard removed after the production check
