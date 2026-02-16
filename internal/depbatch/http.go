// 功能：HTTP 面：对外路由与请求编排。
// 作者：Cason
// 日期：2021.03.31

package depbatch

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	  if rt != nil && rt.Ledger != nil && rt.Wallet != nil {
		   dep := deposit.New(rt.Ledger, rt.Wallet)
		  deposit.MountGuards(dep)
		   if rt.Durable.Recon != nil {
			   dep.BindRecon(rt.Durable.Recon)
		  }
		  svc.BindDeposit(dep)
	   }
	  v1.POST("/depbatch/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
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
	   v1.POST("/depbatch/submit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
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
		   got, err := svc.Submit(c.Request.Context(), body.ID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/depbatch/halt", func(c *gin.Context) {
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
	  v1.GET("/depbatch/list", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"batches": svc.List(50), "host": "deposit-withdraw.kucoin.net"})
	   })
	   v1.GET("/depbatch/:id", func(c *gin.Context) {
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
  }
