package egressleasegate

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

// Handler HTTP。

type Handler struct {
	engine *Engine
	haltWorker *HaltFlipWorker
}

// NewHandler 构造。

  func NewHandler(engine *Engine, haltWorker *HaltFlipWorker) *Handler {
	   return &Handler{engine: engine, haltWorker: haltWorker}
  }

// Register 路由。
   func (h *Handler) Register(rg *gin.RouterGroup) {
	   rg.POST("/egressleasegate/lease/check", h.checkLease)
	  rg.POST("/egressleasegate/broadcast/assert", h.broadcastAssert)
	   rg.POST("/egressleasegate/halt/revoke", h.haltRevoke)
	   rg.POST("/egressleasegate/lease/revoke", h.manualRevoke)
	   rg.GET("/egressleasegate/lease/:lease_id", h.getLease)
	   rg.GET("/egressleasegate/leases", h.listLeases)
	   rg.GET("/egressleasegate/proof/:proof_id", h.getProof)
	  rg.GET("/egressleasegate/proof/:proof_id/export", h.exportProof)
	   rg.GET("/egressleasegate/proofs", h.listProofs)
	   rg.POST("/egressleasegate/proof/confirm", h.confirmProof)
	  rg.POST("/egressleasegate/proof/dismiss", h.dismissProof)
	   rg.GET("/egressleasegate/generations", h.listGens)
	   rg.POST("/egressleasegate/broadcast/batch", h.broadcastBatch)
	   rg.POST("/egressleasegate/water/patrol", h.waterPatrol)
	  rg.POST("/egressleasegate/lease/expire", h.expireDue)
	   rg.GET("/egressleasegate/audit/:lease_id", h.listAudit)
	   rg.GET("/egressleasegate/generation", h.getGeneration)
	   rg.GET("/egressleasegate/metrics", h.metrics)
  }

type checkReq struct {
	Chain string `json:"chain"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	HotAvail string `json:"hot_available"`
	FromTank bool   `json:"from_tank"`
	IdemKey  string `json:"idem_key"`
	Actor string `json:"actor"`
}

  func (h *Handler) checkLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	  var req checkReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	   }
	  res, err := h.engine.CheckEgressWithLease(c.Request.Context(), IssueRequest{
		   Chain: req.Chain, Asset: req.Asset, Amount: req.Amount, HotAvail: req.HotAvail, FromTank: req.FromTank, IdemKey: req.IdemKey, Actor: req.Actor,
	  })
	  if err != nil && (res == nil || !res.Allowed) {
		  status := http.StatusForbidden
		  if errors.Is(err, ErrWarnNotAllowed) || errors.Is(err, ErrRefillActive) || errors.Is(err, ErrCheckDenied) {
			  status = http.StatusForbidden
		   }
		   body := gin.H{"allowed": false, "error": err.Error()}
		  if res != nil {
			  body["reject_reason"] = res.RejectReason
			   body["event"] = res.Event
		  }
		   c.JSON(status, body)
		   return
	   }
	   c.JSON(http.StatusCreated, gin.H{
		  "allowed": true,
		  "egress_lease_id": res.Lease.ID,
		   "state":           res.Lease.State,
		  "breach_level": res.Lease.BreachLevel,
		  "flags_fingerprint": res.Lease.FlagsFingerprint,
		  "flags_fp":        res.Lease.FlagsFingerprint,
		   "tank_digest":     res.Lease.TankDigest,
		   "halt_generation": res.Lease.HaltGeneration,
		  "expires_at":      res.Lease.ExpiresAt,
		   "event": res.Event,
	   })
   }

type assertReq struct {
	LeaseID string `json:"egress_lease_id"`
	LeaseIDAlt string `json:"lease_id"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Amount       string `json:"amount"`
	BroadcastRef string `json:"broadcast_ref"`
	FromTank     bool   `json:"from_tank"`
	HotAvail     string `json:"hot_available"`
}

   func (h *Handler) broadcastAssert(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   abortForbidden(c, err)
		  return
	   }
	   var req assertReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	   res, err := h.engine.BroadcastAssert(c.Request.Context(), BroadcastRequest{
		   LeaseID: firstNonEmpty(req.LeaseID, req.LeaseIDAlt), Chain: req.Chain, Asset: req.Asset, Amount: req.Amount,
		  BroadcastRef: req.BroadcastRef, FromTank: req.FromTank, HotAvail: req.HotAvail,
	  })
	  if err != nil {
		  status := http.StatusForbidden
		  switch {
		  case errors.Is(err, ErrNoLease):
			  status = http.StatusBadRequest
		   case errors.Is(err, ErrLeaseExpired), errors.Is(err, ErrLeaseRevoked), errors.Is(err, ErrLeaseConsumed):
			  status = http.StatusGone
		   case errors.Is(err, ErrNotFound):
			   status = http.StatusNotFound
		  }
		  body := gin.H{"allowed": false, "error": err.Error()}
		   if res != nil {
			  body["reject_reason"] = res.RejectReason
			   if res.DriftProof != nil {
				  body["drift_proof_id"] = res.DriftProof.ID
				  body["drift_type"] = res.DriftProof.DriftType
				  body["proof"] = res.DriftProof
			  }
			   body["accepted"] = false
		  }
		  c.JSON(status, body)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		  "allowed": true,
		   "egress_lease_id": res.Lease.ID,
		   "state":       res.Lease.State,
		   "consumed_at": res.Lease.ConsumedAt,
		   "accepted":    true,
	   })
  }

   func firstNonEmpty(a, b string) string {
	  if a != "" {
		  return a
	  }
	   return b
  }

   func (h *Handler) broadcastBatch(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	  var req BatchRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	  res,
		  err := h.engine.BroadcastBatch(c.Request.Context(), req)
	  if err != nil && res == nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "accepted": false})
		  return
	  }
	   status := http.StatusOK
	  if err != nil {
		   status = http.StatusForbidden
	   }
	   c.JSON(status, res)
   }

   func (h *Handler) waterPatrol(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	  rep, err := h.engine.ObserveWater(c.Request.Context())
	   if err != nil {
		  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, rep)
  }

  func (h *Handler) expireDue(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		  return
	   }
	  n, err := h.engine.ExpireDue(c.Request.Context())
	   if err != nil {
		   c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"expired": n})
   }

   func (h *Handler) listAudit(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  rows, err := h.engine.ListAudit(c.Request.Context(), c.Param("lease_id"), 50)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"audit": rows})
  }

  func (h *Handler) getGeneration(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  g, err := h.engine.Generation(c.Request.Context(), c.Query("chain"), c.Query("asset"))
	   if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, g)
  }

   func (h *Handler) metrics(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, h.engine.Metrics())
   }

type haltReq struct {
	Scope string `json:"scope"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
}

  func (h *Handler) haltRevoke(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	   var req haltReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  if req.Scope == "" {
		   req.Scope = "all"
	  }
	   var (
		  n   int
		   err error
	  )
	   if h.haltWorker != nil {
		   switch req.Scope {
		  case "chain":
			   n, err = h.haltWorker.NotifyHaltChain(c.Request.Context(), req.Chain)
		  case "asset":
			   n, err = h.haltWorker.NotifyHaltAsset(c.Request.Context(), req.Asset)
		  default:
			  n, err = h.haltWorker.NotifyHaltAll(c.Request.Context())
		  }
	  } else {
		  n, err = h.engine.RevokeOnHalt(c.Request.Context(), req.Scope, req.Chain, req.Asset, "ops")
	  }
	  if err != nil {
		   c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"revoked": n, "scope": req.Scope})
   }

type manRevokeReq struct {
	LeaseID string `json:"egress_lease_id"`
	Reason string `json:"reason"`
}

  func (h *Handler) manualRevoke(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	  var req manRevokeReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	  lease,
		   err := h.engine.ManualRevoke(c.Request.Context(), req.LeaseID, req.Reason)
	  if err != nil {
		  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"egress_lease_id": lease.ID, "state": lease.State, "revoke_reason": lease.RevokeReason})
  }

  func (h *Handler) getLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	  lease, err := h.engine.GetLease(c.Request.Context(), c.Param("lease_id"))
	   if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, lease)
  }

   func (h *Handler) listLeases(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   limit := 20
	   if ls := c.Query("limit"); ls != "" {
		  if n, e := strconv.Atoi(ls); e == nil && n > 0 && n <= 200 {
			   limit = n
		   }
	   }
	  rows, err := h.engine.ListActive(c.Request.Context(), c.Query("chain"), c.Query("asset"), limit)
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"leases": rows})
  }

  func (h *Handler) getProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	  p, err := h.engine.GetProof(c.Request.Context(), c.Param("proof_id"))
	   if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, p)
   }

   func (h *Handler) exportProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  exp, err := h.engine.ExportProof(c.Request.Context(), c.Param("proof_id"), "ops")
	   if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, exp)
   }

  func (h *Handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	   state := ProofState(c.Query("state"))
	   if state == "" {
		  state = ProofStateOpen
	  }
	   limit := 20
	  if ls := c.Query("limit"); ls != "" {
		  if n, e := strconv.Atoi(ls); e == nil && n > 0 && n <= 200 {
			  limit = n
		  }
	  }
	  rows,
		   err := h.engine.store.ListProofsByState(c.Request.Context(), state, limit)
	  if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": rows})
  }

type closeProofReq struct {
	ProofID string `json:"proof_id"`
	ClosedBy string `json:"closed_by"`
	Notes string `json:"notes"`
}

  func (h *Handler) confirmProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	  var req closeProofReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	  if err := h.engine.ConfirmProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes); err != nil {
		  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"proof_id": req.ProofID, "state": "closed"})
  }

  func (h *Handler) dismissProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		  return
	   }
	  var req closeProofReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	   }
	   if err := h.engine.DismissProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes); err != nil {
		  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"proof_id": req.ProofID, "state": "closed"})
   }

   func (h *Handler) listGens(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   rows, err := h.engine.store.ListGenerations(c.Request.Context(), 50)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"generations": rows})
  }
