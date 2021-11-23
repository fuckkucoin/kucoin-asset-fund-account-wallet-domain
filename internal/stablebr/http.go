package stablebr

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
	   v1.POST("/stablebr/attest", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body AttestReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Attest(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/stablebr/mint", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
			   return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/stablebr/redeem", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body RedeemReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Redeem(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	  v1.GET("/stablebr/journal", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{
			   "rows":          svc.List(c.Query("user_id"), 50),
			  "host":          "ledger-transfer-freeze-reconciliation.kucoin.net",
			  "mint_lease_id": c.Query("mint_lease_id"),
		  })
	   })
	  v1.GET("/stablebr/checklist", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  rows, err := svc.Checklist(c.Query("kind"), c.Query("asset"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"checks": rows})
	  })
	   v1.POST("/stablebr/attest-clear", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Kind  string `json:"kind"`
			  Asset string `json:"asset"`
			  Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if err := svc.ClearAttest(body.Kind, body.Asset, body.Actor); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"cleared": true})
	  })
  }
