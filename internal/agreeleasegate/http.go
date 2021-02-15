package agreeleasegate

import (
	"errors"
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func abortForbidden(c *gin.Context, err error) {
	   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
   }

  func abortBadRequest(c *gin.Context, err error) {
	  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
   }

   func abortConflict(c *gin.Context, err error) {
	  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
   }

// Handler HTTP。
type Handler struct {
	engine *Engine
	drift *DriftRevokeWorker
}

// NewHandler 构造。
   func NewHandler(engine *Engine, drift *DriftRevokeWorker) *Handler {
	  return &Handler{engine: engine, drift: drift}
   }

// Register 路由。
   func (h *Handler) Register(rg *gin.RouterGroup) {
	   rg.POST("/agreeleasegate/lease/agree", h.agreeLease)
	  rg.POST("/agreeleasegate/plan/assert", h.planAssert)
	   rg.POST("/agreeleasegate/exec/assert", h.execAssert)
	   rg.POST("/agreeleasegate/lease/revoke", h.manualRevoke)
	  rg.POST("/agreeleasegate/bal/revoke", h.balRevoke)
	   rg.POST("/agreeleasegate/rule/revoke", h.ruleRevoke)
	  rg.GET("/agreeleasegate/lease/:lease_id", h.getLease)
	   rg.GET("/agreeleasegate/leases", h.listLeases)
	  rg.GET("/agreeleasegate/proof/:proof_id", h.getProof)
	  rg.GET("/agreeleasegate/proof/:proof_id/export", h.exportProof)
	   rg.GET("/agreeleasegate/proofs", h.listProofs)
	   rg.POST("/agreeleasegate/proof/confirm", h.confirmProof)
	   rg.POST("/agreeleasegate/proof/dismiss", h.dismissProof)
	   rg.GET("/agreeleasegate/generations", h.listGens)
	  rg.GET("/agreeleasegate/half-plans", h.listHalf)
	  rg.GET("/agreeleasegate/metrics", h.metrics)
   }

type agreeReq struct {
	Chain string `json:"chain"`
	Asset   string `json:"asset"`
	IdemKey string `json:"idem_key"`
}

   func (h *Handler) agreeLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	   var req agreeReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	  }
	  res, err := h.engine.AgreeWithLease(c.Request.Context(), IssueRequest(req))
	   if err != nil {
		   if errors.Is(err, ErrAgreeDenied) && res != nil {
			  c.JSON(http.StatusConflict, gin.H{"allowed": false, "reason": res.RejectReason, "event": res.Event})
			   return
		  }
		  abortConflict(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"allowed": res.Allowed, "lease": res.Lease, "event": res.Event})
   }

type planReq struct {
	LeaseID string `json:"agree_lease_id"`
	Chain   string `json:"chain"`
	Asset   string `json:"asset"`
	PlanRef string `json:"plan_ref"`
}

  func (h *Handler) planAssert(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	   var req planReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	   res, err := h.engine.PlanAssertLease(c.Request.Context(), PlanAssertRequest(req))
	   writeAssert(c, res, err)
  }

type execReq struct {
	LeaseID string `json:"agree_lease_id"`
	Chain   string `json:"chain"`
	Asset   string `json:"asset"`
	ExecRef string `json:"exec_ref"`
	Consume bool   `json:"consume"`
}

  func (h *Handler) execAssert(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   abortForbidden(c, err)
		  return
	  }
				   body["lease"] = res.Lease
			   }
		  }
		   c.JSON(http.StatusConflict, body)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"allowed": true, "lease": res.Lease})
  }

type revokeReq struct {
	LeaseID string `json:"agree_lease_id"`
	Reason  string `json:"reason"`
	Asset   string `json:"asset"`
}

  func (h *Handler) manualRevoke(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	   var req revokeReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	  }
	   l, err := h.engine.ManualRevoke(c.Request.Context(), req.LeaseID, req.Reason)
	   if err != nil {
		  abortConflict(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, l)
  }

  func (h *Handler) balRevoke(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	  var req revokeReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	  }
	   n, err := h.engine.RevokeOnBalCross(c.Request.Context(), req.Asset, req.Reason)
	   if err != nil {
		   abortConflict(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"revoked": n})
   }

  func (h *Handler) ruleRevoke(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	   var req revokeReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	   }
	  n, err := h.engine.RevokeOnRuleFlip(c.Request.Context(), req.Asset, req.Reason)
	   if err != nil {
		   abortConflict(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"revoked": n})
   }

  func (h *Handler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	  l, err := h.engine.GetLease(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, l)
   }

  func (h *Handler) listLeases(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  rows, err := h.engine.ListActive(c.Request.Context(), c.Query("asset"), limit)
	  if err != nil {
		  abortBadRequest(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"leases": rows})
  }

   func (h *Handler) getProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	   p, err := h.engine.GetProof(c.Request.Context(), c.Param("proof_id"))
	  if err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, p)
  }

  func (h *Handler) exportProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  out, err := h.engine.ExportProof(c.Request.Context(), c.Param("proof_id"), "ops")
	  if err != nil {
		  abortBadRequest(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, out)
  }

   func (h *Handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	  st := ProofState(c.DefaultQuery("state", "open"))
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   rows, err := h.engine.ListProofs(c.Request.Context(), st, limit)
	   if err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": rows})
   }

type proofCloseReq struct {
	ProofID  string `json:"proof_id"`
	ClosedBy string `json:"closed_by"`
	Notes    string `json:"notes"`
}

   func (h *Handler) confirmProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  var req proofCloseReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	   if err := h.engine.ConfirmProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes); err != nil {
		  abortConflict(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"ok": true})
   }

   func (h *Handler) dismissProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	  var req proofCloseReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  if err := h.engine.DismissProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes); err != nil {
		   abortConflict(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"ok": true})
  }

   func (h *Handler) listGens(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  rows, err := h.engine.ListGenerations(c.Request.Context(), limit)
	  if err != nil {
		   abortBadRequest(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"generations": rows})
   }

  func (h *Handler) listHalf(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   rows,
		   err := h.engine.ListHalfPlans(c.Request.Context(), limit)
	  if err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"half_plans": rows})
   }

  func (h *Handler) metrics(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, SnapshotMetrics())
  }
