package fundguard

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   if rt != nil && rt.Ledger != nil {
		   svc.BindLedger(rt.Ledger)
	   }
	  v1.POST("/fundguard/gate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body Check
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "").ID
		   }
		   got, err := svc.Gate(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/fundguard/hash", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body HashClaim
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.ClaimHash(body.Chain, body.TxHash, body.Idem)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/fundguard/partial", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body Partial
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := svc.CheckPartial(body); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true})
	  })
	   v1.POST("/fundguard/halt", func(c *gin.Context) {
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
	  v1.GET("/fundguard/snaps", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"snaps": svc.Snaps(), "host": "custody-boundary-lab.kucoin.net"})
	   })
  }
