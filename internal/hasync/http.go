package hasync

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil {
		   if rt.Redis != nil {
			   svc.BindPair(ha.PairFromEnv("ledger", ha.RedisBackend{R: rt.Redis}))
		  } else {
			  svc.BindPair(ha.PairFromEnv("ledger", ha.NewMemBackend()))
		  }
		  _, code := rt.Ready(context.Background())
		  _ = svc.WireReady(context.Background(), code)
	  }
	  v1.GET("/hasync/probe", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"probe": svc.ProbeNow(c.DefaultQuery("region", "sg")), "host": config.HostQueryGateway})
	   })
	   v1.POST("/hasync/campaign", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.Campaign(c.Request.Context(), body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/hasync/write", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body WriteReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.GuardWrite(c.Request.Context(), body.Actor, func() error { return nil })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.POST("/hasync/failover", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  Actor  string `json:"actor"`
			  Reason string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.Failover(c.Request.Context(), body.Actor, body.Reason)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/hasync/ready", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  On bool `json:"on"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   svc.SetReady(body.On)
		  c.JSON(200, gin.H{"ready": svc.Ready()})
	   })
	  v1.GET("/hasync/history", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"history": svc.History(50)})
	  })
   }
