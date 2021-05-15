package fiatjnl

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   if rt != nil {
		   svc.BindLedger(rt.Ledger)
	   }
	   _ = rt
	  v1.POST("/fiatjnl/kyc", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   UserID string `json:"user_id"`
			   OK bool `json:"ok"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := svc.MarkKYC(body.UserID, body.OK); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"user_id": body.UserID, "kyc": body.OK})
	   })
	   v1.POST("/fiatjnl/post", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body PostReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  body.UserID = uid
		  if rt != nil && !rt.Mem && rt.Ledger == nil {
			  c.JSON(503, gin.H{"error": "ledger unavailable"})
			  return
		  }
		   got, err := svc.Post(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	   v1.GET("/fiatjnl/recent", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"entries": svc.List(c.Query("user_id"), 50), "host": "deposit-withdraw.kucoin.net"})
	   })
  }
