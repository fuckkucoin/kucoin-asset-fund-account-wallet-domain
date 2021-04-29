// HTTP 面：对外路由与请求编排。

// Author: Cason
// Date: 2021.04.28

package feecredit

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
	  v1.POST("/feecredit/post", func(c *gin.Context) {
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
		   got,
			   err := svc.Post(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/feecredit/recent", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{
			  "credits": svc.List(c.Query("user_id"), 50),
			   "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		  })
	   })
   }
