package deppartial

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 注册 HTTP 路由。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   _ = rt
	   g := v1.Group("/deppartial")
	   g.POST("/scan", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body ScanInput
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   result, err := svc.Scan(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, result)
	   })
	   g.GET("/residues", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   batchID := c.Query("batch_id")
		   residues,
			   err := svc.ListResidues(batchID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"residues": residues})
	   })
	  g.GET("/view", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  batchID := c.Query("batch_id")
		   view, err := svc.GetView(batchID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, view)
	  })
	   g.POST("/reopen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body ReopenReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   lease, err := svc.AcquireLease(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, lease)
	   })
	   g.POST("/repair/credit", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body RepairCreditReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  result, err := svc.RepairCredit(body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, result)
	   })
	   g.POST("/repair/abandon", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body AbandonReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  result, err := svc.Abandon(body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, result)
	   })
	  g.POST("/repair/escalate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body EscalateReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  result, err := svc.Escalate(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, result)
	   })
	  g.POST("/proof/check", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body AlignCheckReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  proof, err := svc.CheckAlign(body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, proof)
	   })
	  g.POST("/proof/check-batch", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   BatchID string `json:"batch_id"`
			  Checker string `json:"checker"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  proofs, err := svc.CheckAlignBatch(body.BatchID, body.Checker)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"proofs": proofs})
	  })
	   g.POST("/proof/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body ProofExportReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  resp, err := svc.ExportProofs(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, resp)
	  })
	  g.POST("/close", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body CloseReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   view, err := svc.CloseBatch(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, view)
	   })
	   g.POST("/force-close", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  BatchID string `json:"batch_id"`
			  Actor string `json:"actor"`
			   Approver string `json:"approver"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  view, err := svc.ForceClose(body.BatchID, body.Actor, body.Approver)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, view)
	   })
	   g.POST("/lease/release", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   LeaseID string `json:"lease_id"`
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   lease,
			  err := svc.ReleaseLease(body.LeaseID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, lease)
	  })
	   g.GET("/lease/active", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  batchID := c.Query("batch_id")
		  lease, err := svc.GetActiveLease(batchID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, lease)
	  })
	   g.GET("/fence/check", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  chain := c.Query("chain")
		   txHash := c.Query("tx_hash")
		   fence, err := svc.CheckFence(chain, txHash)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, fence)
	  })
	   g.GET("/fence/list", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   batchID := c.Query("batch_id")
		  fences, err := svc.ListFences(batchID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"fences": fences})
	   })
	  g.GET("/can-close", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  batchID := c.Query("batch_id")
		  ok,
			  reason, err := svc.CanClose(batchID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"can_close": ok, "reason": reason})
	  })
   }
