package p2pgate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/p2pnet"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil {
		  svc.BindLedger(rt.Ledger)
	   }
	  v1.POST("/p2pgate/send", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body p2pnet.SendReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.Send(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	  v1.GET("/p2pgate/recent", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"recent": svc.Recent(50), "host": "account-book.kucoin.net"})
	   })
   }
