// 功能：HTTP 面：对外路由与请求编排。
// 作者：Bronson
// 日期：2021.10.27

package safesaga

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
	   v1.POST("/safesaga/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			   Amount string `json:"amount"`
			   Idem   string `json:"idempotency_key"`
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
		   got, err := svc.Open(body.UserID, body.Asset, body.Amount, body.Idem)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/safesaga/advance", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  ID string `json:"id"`
			   Actor string `json:"actor"`
			  Epoch int64  `json:"epoch"`
			   Token string `json:"token"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  if actor == "" {
			  actor = body.Actor
		  }
		  got, err := svc.Advance(c.Request.Context(), body.ID, actor, body.Epoch, body.Token)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/safesaga/runs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"runs": svc.List(), "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
   }
