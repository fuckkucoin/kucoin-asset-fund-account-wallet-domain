package p2pnet

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  if rt != nil {
		  svc.BindLedger(rt.Ledger)
	   }
	   v1.POST("/p2p/send", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body SendReq
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
	   v1.POST("/p2p/reverse", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   ID    string `json:"id"`
			   Actor string `json:"actor"`
			  Reason string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.Reverse(c.Request.Context(), body.ID, body.Actor, body.Reason)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/p2p/block", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if err := svc.Risk().Block(body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"blocked": body.UserID})
	  })
	  v1.GET("/p2p/transfers", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{
			   "transfers": svc.List(c.Query("user_id"), 50),
			  "host":      "account-book.kucoin.net",
		   })
	   })
  }
