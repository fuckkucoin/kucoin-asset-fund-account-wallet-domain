package ratelimit

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  q := NewQuery(8, 2)
	  w := NewWithdraw(3, 0.05)
	  fallback := ""
	   if rt != nil && rt.Mem && rt.Cfg != nil {
		  fallback = rt.Cfg.SeedUserID
	  }
	  v1.GET("/rate/query", QueryMW(q, fallback), func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"ok": true, "host": "iam-gateway.kucoin.net"})
	  })
	  v1.POST("/rate/withdraw", WithdrawMW(w, fallback), func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	  })
	  v1.GET("/rate/remaining", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   actor := c.DefaultQuery("actor", fallback)
		  qk, _ := ActorKey(actor, ActionQuery)
		   wk, _ := ActorKey(actor, ActionWithdraw)
		  c.JSON(200, gin.H{"query": q.Table().Remaining(qk), "withdraw": w.Table().Remaining(wk)})
	   })
   }
