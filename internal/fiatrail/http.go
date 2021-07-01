package fiatrail

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil {
		   svc.BindLedger(rt.Ledger)
	   }
	   v1.POST("/fiat/kyc", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body Profile
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := svc.KYC().Put(body); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, body)
	   })
	   v1.POST("/fiat/book", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body BookReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.Book(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/fiat/kyc/screen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   ID string `json:"id"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got,
			   err := svc.ScreenKYC(body.ID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	   v1.POST("/fiat/authorize", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  ID string `json:"id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got,
			  err := svc.Authorize(body.ID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	   v1.POST("/fiat/settle", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  ID string `json:"id"`
			  Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-fiat").ID
		  }
		  got, err := svc.Settle(c.Request.Context(), body.ID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/fiat/refund", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  ID     string `json:"id"`
			  Actor  string `json:"actor"`
			  Reason string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got,
			   err := svc.Refund(c.Request.Context(), body.ID, body.Actor, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	   v1.GET("/fiat/bookings", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{
			   "bookings": svc.List(c.Query("user_id"), 50),
			  "host":     "deposit-withdraw.kucoin.net",
		  })
	   })
   }
