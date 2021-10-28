package riskpack

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		   svc.BindSQL(rt.SQL)
	   }
	  v1.POST("/risk/caps", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body Cap
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if err := svc.SetCap(body); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, body)
	  })
	   v1.POST("/risk/evaluate", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			  Chain  string `json:"chain"`
			  Dest   string `json:"dest"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  body.UserID = uid
		  res, err := svc.Check(Request{UserID: body.UserID, Asset: body.Asset, Amount: body.Amount, Chain: body.Chain, Dest: body.Dest})
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, res)
	  })
	   v1.GET("/risk/escalate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"items": svc.Pending(), "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	   v1.POST("/risk/escalate/:id/decide", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  Decision string `json:"decision"`
			  Epoch    int64  `json:"epoch"`
			  Token    string `json:"token"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if _, err := validate.Actor(actor.ID); err != nil {
			  mw.WriteProblem(c, ErrActor)
			  return
		  }
		   it, err := svc.Decide(c.Param("id"), actor.ID, body.Decision, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, it)
	   })
  }
