package ledbatch

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  if rt != nil {
		   svc.BindLedger(rt.Ledger)
	   }
	  v1.POST("/ledbatch/open", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
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
	   v1.POST("/ledbatch/submit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
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
	   v1.POST("/ledbatch/halt", func(c *gin.Context) {
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
	   v1.GET("/ledbatch/list", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"batches": svc.List(50), "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	   })
	  v1.GET("/ledbatch/:id", func(c *gin.Context) {
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
	   v1.POST("/ledbatch/cancel", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
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
		  got, err := svc.Cancel(body.ID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
   }
