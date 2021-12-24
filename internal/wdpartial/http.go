package wdpartial

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// registerHTTP 注册所有 wdpartial HTTP 端点。

  func registerHTTP(v1 *gin.RouterGroup, svc *Service) {
	   // ── 残留检测 ───────────────────────────────────────────────────────────────
	   v1.POST("/wdpartial/detect", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   BatchID string `json:"batch_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   result, err := svc.DetectResidues(body.BatchID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, result)
	  })
	   v1.GET("/wdpartial/residues", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  batchID := c.Query("batch_id")
		  items, err := svc.ListResidues(ResidueFilter{WDBatchID: batchID})
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items)})
	   })
	  v1.GET("/wdpartial/residues/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  item, err := svc.GetResidue(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, item)
	  })
	   // ── Resume 世代锁 ──────────────────────────────────────────────────────────
	   v1.POST("/wdpartial/lease/acquire", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  BatchID string `json:"batch_id"`
			  Holder  string `json:"holder"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   lease, err := svc.AcquireResumeLease(body.BatchID, body.Holder)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, lease)
	   })
	  v1.POST("/wdpartial/lease/verify", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   BatchID string `json:"batch_id"`
			   Holder  string `json:"holder"`
			   Gen int64 `json:"gen"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.VerifyLease(body.BatchID, body.Holder, body.Gen); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"ok": true})
	  })
	  v1.POST("/wdpartial/lease/release", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   BatchID string `json:"batch_id"`
			   Holder  string `json:"holder"`
			  Gen     int64  `json:"gen"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if err := svc.ReleaseLease(body.BatchID, body.Holder, body.Gen); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"released": true})
	  })
	  v1.GET("/wdpartial/lease/:batch_id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  lease, err := svc.CurrentLease(c.Param("batch_id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, lease)
	   })
	   // ── 释放证明 ──────────────────────────────────────────────────────────────
	  v1.POST("/wdpartial/proof/record", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   BatchID   string           `json:"batch_id"`
			   ResidueID string           `json:"residue_id"`
			  Kind      ReleaseProofKind `json:"kind"`
			  Actor     string           `json:"actor"`
			   LedgerRef string           `json:"ledger_ref"`
			  Summary     string `json:"summary"`
			  LeaseHolder string `json:"lease_holder"`
			   LeaseGen int64 `json:"lease_gen"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   proof, err := svc.RecordReleaseProof(RecordProofReq{
			  WDBatchID: body.BatchID,
			  ResidueID: body.ResidueID,
			  Kind:  body.Kind,
			  Actor: body.Actor,
			   LedgerRef: body.LedgerRef,
			   Summary: body.Summary,
			  LeaseHolder: body.LeaseHolder,
			   LeaseGen: body.LeaseGen,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, proof)
	  })
	  v1.POST("/wdpartial/proof/mark-retried", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  ResidueID    string `json:"residue_id"`
			  RetryBatchID string `json:"retry_batch_id"`
			   LeaseHolder  string `json:"lease_holder"`
			   LeaseGen int64 `json:"lease_gen"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   if err := svc.MarkRetried(body.ResidueID, body.RetryBatchID, body.LeaseHolder, body.LeaseGen); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"ok": true})
	   })
	  // ── Conserve 收口 ──────────────────────────────────────────────────────────
	   v1.POST("/wdpartial/conserve/check", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body BatchSummary
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  cr, err := svc.CheckConserve(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, cr)
	   })
	   v1.POST("/wdpartial/closeout", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body CloseOutReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   co, err := svc.CloseOut(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, co)
	  })
	  v1.GET("/wdpartial/closeout/:batch_id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   co, err := svc.GetCloseout(c.Param("batch_id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, co)
	   })
   }
