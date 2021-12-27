package wdbatch

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   if rt != nil {
		  svc.BindLedger(rt.Ledger)
		   svc.BindWithdraw(withdraw.New(rt.Ledger))
	  }
	   v1.POST("/wdbatch/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body OpenReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Open(body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/wdbatch/submit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   ID    string `json:"id"`
			  Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Submit(c.Request.Context(), body.ID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/wdbatch/halt", func(c *gin.Context) {
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
		   svc.SetHalt(body.On)
		   c.JSON(200, gin.H{"halt": svc.Halted()})
	  })
	  v1.GET("/wdbatch/list", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"batches": svc.List(50), "host": "deposit-withdraw.kucoin.net"})
	   })
	  registerMore(v1, svc)
	  attachFenceOnRegister(v1, svc)
   }

   func registerMore(v1 *gin.RouterGroup, svc *Service) {
	  v1.GET("/wdbatch/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   got, err := svc.Get(c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/wdbatch/cancel", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
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
		   got,
			  err := svc.Cancel(body.ID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/wdbatch/retry", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  ID             string `json:"id"`
			   Actor          string `json:"actor"`
			  IdempotencyKey string `json:"idempotency_key"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.Retry(body.ID, body.Actor, body.IdempotencyKey)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/wdbatch/sweep-release", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Actor string `json:"actor"`
			  Limit int    `json:"limit"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  rows, err := svc.SweepRelease(c.Request.Context(), body.Actor, body.Limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"rows": rows, "unfreeze_live_hash": false})
	  })
  }
