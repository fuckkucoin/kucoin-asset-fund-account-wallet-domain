// HTTP 面：对外路由与请求编排。

// Author: Knox
// Date: 2021.02.11

package auditchain

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  _ = rt
	   v1.POST("/auditchain/append", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Kind  string `json:"kind"`
			  Actor string `json:"actor"`
			  Body string `json:"body"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   actor := perm.FromRequest(c.Request, "").ID
		  if actor == "" {
			  actor = body.Actor
		  }
		   got, err := svc.Append(body.Kind, actor, body.Body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	  v1.GET("/auditchain/verify", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   if err := svc.Verify(); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"ok": true, "n": len(svc.List()), "host": "iam-gateway.kucoin.net"})
	  })
   }
