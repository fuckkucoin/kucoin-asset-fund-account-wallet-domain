package wdpath

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/wdrelease"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil {
		  if rt.KV != nil {
			   svc.BindKV(rt.KV)
		   }
		   if desk := travelvasp.DeskFrom(rt); desk != nil {
			   svc.travel = desk
		   }
		  if rt.Cfg != nil {
			   svc.BindTravelCodes(rt.Cfg.Travel.OriginVASP, rt.Cfg.Travel.PeerVASP)
			  svc.SetMaxHits(rt.Cfg.Risk.MaxWithdrawVelocity)
		  }
		   if desk := wdrelease.DeskFrom(rt); desk != nil {
			   svc.BindRelease(desk)
		   }
		  svc.BindRuntimeLimit(rt)
		  if !rt.Mem && rt.SQL != nil {
			   if allow := svc.Allow(); allow != nil {
				  kind, profile := "", ""
				   if rt.Cfg != nil {
					  kind, profile = rt.Cfg.Auth.Kind, rt.Cfg.Profile
				   }
				  destallow.BindProd(allow, rt.SQL, kind, profile)
			   }
		   }
	  }
	  v1.POST("/wdpath/allow", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  UserID  string `json:"user_id"`
			  Chain   string `json:"chain"`
			   Address string `json:"address"`
			   Label   string `json:"label"`
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
		  got, err := svc.Allow().AddStrict(body.UserID, body.Chain, body.Address, body.Label)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/wdpath/open", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body OpenReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   body.UserID = uid
		  got, err := svc.Open(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	   v1.GET("/wdpath/recent", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"intents": svc.List(c.Query("user_id"), 50), "host": config.HostDepositWithdraw})
	   })
  }
