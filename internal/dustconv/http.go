package dustconv

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	  _ = rt
	  v1.POST("/dust/convert", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body ConvertReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-dust").ID
		  }
		  got, err := svc.Convert(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/dust/policy", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body Policy
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := svc.PutPolicy(body); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, body)
	   })
	  v1.GET("/dust/scan", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"dust": svc.Scan(c.Query("user_id")), "host": "account-book.kucoin.net"})
	  })
  }
