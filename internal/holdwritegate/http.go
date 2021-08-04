// @FileName: http.go
// @Summary: 冻结巡检漂移建议升级写路径挡写世代引擎 — HTTP 路由。
// @Author: Orin
// @Created: 2021.07.20

package holdwritegate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	   v1.POST("/holdwritegate/escalate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var req EscalateAdviceReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		   esc, lease, err := eng.EscalateAdvice(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"escalation": esc, "lease": lease})
	  })
	   v1.POST("/holdwritegate/write-gate/check", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var req WriteGateCheckReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   resp, err := eng.CheckWriteGate(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, resp)
	  })
	  v1.POST("/holdwritegate/proof/submit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req SubmitCloseProofReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   proof, err := eng.SubmitCloseProof(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, proof)
	  })
	   v1.POST("/holdwritegate/proof/validate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var req ValidateProofReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		  proof, err := eng.ValidateProof(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, proof)
	  })
	   v1.POST("/holdwritegate/clear", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req DualControlClearReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		   lease, err := eng.DualControlClear(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, lease)
	  })
	   v1.POST("/holdwritegate/orphan/bind", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var req BindOrphanTagReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   bind, err := eng.BindOrphanTag(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, bind)
	  })
	  v1.POST("/holdwritegate/lease/expire", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   LeaseID string `json:"lease_id"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		   lease, err := eng.ExpireLease(c.Request.Context(), body.LeaseID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, lease)
	  })
	   v1.POST("/holdwritegate/proof/seal", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   ProofID string `json:"proof_id"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		   proof, err := eng.SealProof(c.Request.Context(), body.ProofID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, proof)
	  })
	  // ──── 查询路由 ────
	   v1.GET("/holdwritegate/block/query", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  uid := c.Query("user_id")
		  leases,
			   err := eng.BlockQuery(c.Request.Context(), BlockQueryReq{UserID: uid})
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, leases)
	   })
	   v1.GET("/holdwritegate/block-gen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   uid := c.Query("user_id")
		   asset := c.Query("asset")
		   book := c.Query("book")
		   resp, err := eng.BlockGenQuery(c.Request.Context(), uid, asset, book)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, resp)
	  })
	  v1.GET("/holdwritegate/lease", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  lid := c.Query("lease_id")
		  if lid != "" {
			  lease, err := eng.GetBlockLease(c.Request.Context(), lid)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			  c.JSON(http.StatusOK, lease)
			  return
		  }
		  c.JSON(http.StatusBadRequest, gin.H{"error": "lease_id required"})
	  })
	  v1.GET("/holdwritegate/escalation/list", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   leaseID := c.Query("lease_id")
		  userID := c.Query("user_id")
		  state := c.Query("state")
		  escs, err := eng.EscalationQuery(c.Request.Context(), EscalationQueryReq{
			   LeaseID: leaseID,
			   UserID: userID,
			  State: state,
			   Limit: 50,
		   })
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, escs)
	  })
	  v1.GET("/holdwritegate/proof/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  leaseID := c.Query("lease_id")
		   userID := c.Query("user_id")
		  proofs, err := eng.ProofExport(c.Request.Context(), ProofExportReq{
			  LeaseID: leaseID,
			   UserID: userID,
			   Limit: 100,
		  })
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, proofs)
	   })
	  v1.GET("/holdwritegate/write-gate/list", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   leaseID := c.Query("lease_id")
		   entries,
			   err := eng.ListWriteGateEntries(c.Request.Context(), leaseID, 100)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, entries)
	  })
	  v1.GET("/holdwritegate/orphan/list", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   leaseID := c.Query("lease_id")
		   binds, err := eng.ListOrphanBinds(c.Request.Context(), leaseID, 100)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, binds)
	   })
	   v1.GET("/holdwritegate/blocking/list", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   leases, err := eng.ListBlockingLeases(c.Request.Context(), 100)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, leases)
	   })
	   v1.GET("/holdwritegate/escalation", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  eid := c.Query("escalation_id")
		  if eid != "" {
			   esc, err := eng.GetEscalation(c.Request.Context(), eid)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(http.StatusOK, esc)
			  return
		  }
		  c.JSON(http.StatusBadRequest, gin.H{"error": "escalation_id required"})
	  })
	  v1.GET("/holdwritegate/proof", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   pid := c.Query("proof_id")
		  if pid != "" {
			  proof, err := eng.GetCloseProof(c.Request.Context(), pid)
			   if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(http.StatusOK, proof)
			   return
		   }
		  c.JSON(http.StatusBadRequest, gin.H{"error": "proof_id required"})
	  })
   }
