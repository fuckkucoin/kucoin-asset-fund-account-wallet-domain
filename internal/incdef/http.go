package incdef

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil {
		   svc.BindLedger(rt.Ledger)
		   svc.BindWithdraw(withdraw.New(rt.Ledger))
	  }
	  v1.GET("/incdef/status", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, svc.Status())
	  })
	  v1.GET("/incdef/timeline", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, svc.ExportTimeline())
	   })
	  v1.POST("/incdef/kill", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  On    bool   `json:"on"`
			  Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  if actor == "" {
			   actor = body.Actor
		   }
		   if err := svc.Kill(actor, body.On); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, svc.Status())
	  })
	   v1.POST("/incdef/degrade", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  On    bool   `json:"on"`
			  Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  if actor == "" {
			   actor = body.Actor
		  }
		  if err := svc.Degrade(actor, body.On); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, svc.Status())
	  })
	  v1.POST("/incdef/freeze", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Actor string `json:"actor"`
			   Reason string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  actor := perm.FromRequest(c.Request, "").ID
		  if actor == "" {
			   actor = body.Actor
		  }
		  if err := svc.Freeze(actor, body.Reason); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, svc.Status())
	   })
	   v1.POST("/incdef/drift", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  UserID string `json:"user_id"`
			  Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		   if actor == "" {
			  actor = body.Actor
		  }
		  if err := svc.FreezeOnDrift(c.Request.Context(), actor, body.UserID); err != nil && err != ErrDrift {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, svc.Status())
	  })
	   v1.GET("/incdef/alerts", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"alerts": svc.Alerts(), "runbook": svc.Status().Runbook})
	   })
	  v1.GET("/incdef/timeline/hash", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, svc.HashTimeline())
	   })
	  v1.POST("/incdef/stuck", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  TransferID string `json:"transfer_id"`
			  Reason     string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.OpenStuck(body.TransferID, body.Reason)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
   }
