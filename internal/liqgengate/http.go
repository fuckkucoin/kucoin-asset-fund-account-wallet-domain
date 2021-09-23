package liqgengate

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
}

// NewHandler 构造。
  func NewHandler(engine *Engine) *Handler { return &Handler{engine: engine} }

// Register 路由。

   func (h *Handler) Register(rg *gin.RouterGroup) {
	  rg.POST("/liqgengate/freeze", h.freeze)
	   rg.POST("/liqgengate/release", h.release)
	   rg.POST("/liqgengate/close", h.closeBorrow)
	  rg.POST("/liqgengate/ops/force-close", h.forceClose)
	   rg.GET("/liqgengate/generation/:borrow_id", h.getGen)
	   rg.GET("/liqgengate/generations", h.listGens)
	   rg.GET("/liqgengate/lease/:lease_id", h.getLease)
	   rg.GET("/liqgengate/leases", h.listLeases)
	   rg.GET("/liqgengate/proof/:proof_id", h.getProof)
	  rg.GET("/liqgengate/proof/:proof_id/export", h.exportProof)
	  rg.GET("/liqgengate/proofs", h.listProofs)
	   rg.GET("/liqgengate/proofs/half", h.listHalf)
	   rg.GET("/liqgengate/proofs/race", h.listRace)
	  rg.POST("/liqgengate/proof/confirm", h.confirmProof)
	   rg.POST("/liqgengate/proof/dismiss", h.dismissProof)
	  rg.GET("/liqgengate/audit/:borrow_id", h.listAudit)
	  rg.GET("/liqgengate/metrics", h.metrics)
  }

type freezeReq struct {
	UserID   string `json:"user_id"`
	BorrowID string `json:"borrow_id"`
	IdemKey string `json:"idem_key"`
}

  func (h *Handler) freeze(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   var req freezeReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	  }
	   res, err := h.engine.FreezeWithGen(c.Request.Context(), FreezeRequest(req))
	  if err != nil {
		  body := gin.H{"allowed": false, "error": err.Error()}
		   if res != nil {
			  body["reason"] = res.RejectReason
			  if res.HalfProof != nil {
				   body["half_proof"] = res.HalfProof
			  }
			   if res.Generation != nil {
				  body["generation"] = res.Generation
			   }
			   if res.Lease != nil {
				  body["lease"] = res.Lease
			   }
		   }
		  c.JSON(http.StatusConflict, body)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"allowed": true, "generation": res.Generation, "lease": res.Lease, "hold_id": res.HoldID})
  }

type releaseReq struct {
	UserID   string `json:"user_id"`
	BorrowID string `json:"borrow_id"`
	HoldID string `json:"hold_id"`
	LiqGen  *uint64 `json:"liq_gen"`
	LeaseID string  `json:"lease_id"`
	IdemKey string `json:"idem_key"`
}

  func (h *Handler) release(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   var req releaseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	   in := ReleaseRequest{UserID: req.UserID, BorrowID: req.BorrowID, HoldID: req.HoldID, LeaseID: req.LeaseID, IdemKey: req.IdemKey}
	   if req.LiqGen != nil {
		   in.HasGen = true
		  in.LiqGen = *req.LiqGen
	   }
	   res, err := h.engine.ReleaseWithGen(c.Request.Context(), in)
	   writeGate(c, res, err)
  }

type closeReq struct {
	UserID string `json:"user_id"`
	BorrowID string `json:"borrow_id"`
	LiqGen  *uint64 `json:"liq_gen"`
	IdemKey string  `json:"idem_key"`
}

  func (h *Handler) closeBorrow(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   var req closeReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	  }
	   in := CloseRequest{UserID: req.UserID, BorrowID: req.BorrowID, IdemKey: req.IdemKey}
	   if req.LiqGen != nil {
		  in.HasGen = true
		  in.LiqGen = *req.LiqGen
	   }
	  res,
		  err := h.engine.CloseWithGen(c.Request.Context(), in)
	  writeGate(c, res, err)
   }

  func writeGate(c *gin.Context, res *GateResult, err error) {
	  if err != nil {
		   body := gin.H{"allowed": false, "error": err.Error()}
		   if res != nil {
			  body["reason"] = res.RejectReason
			  if res.RaceProof != nil {
				  body["race_proof"] = res.RaceProof
			   }
			  if res.Generation != nil {
				  body["generation"] = res.Generation
			   }
		   }
		  c.JSON(http.StatusConflict, body)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"allowed": true, "generation": res.Generation, "lease": res.Lease})
   }

type forceReq struct {
	BorrowID string `json:"borrow_id"`
	Note string `json:"note"`
}

  func (h *Handler) forceClose(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  var req forceReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	  g, err := h.engine.ForceClose(c.Request.Context(), req.BorrowID, req.Note)
	   if err != nil {
		   abortConflict(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, g)
  }

   func (h *Handler) getGen(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	   g, err := h.engine.GetGeneration(c.Request.Context(), c.Param("borrow_id"))
	   if err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, g)
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
	  c.JSON(http.StatusOK, gin.H{"generations": rows, "host": "account-book.kucoin.net"})
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
	  rows, err := h.engine.ListActive(c.Request.Context(), limit)
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
	  ex, err := h.engine.ExportProof(c.Request.Context(), c.Param("proof_id"))
	  if err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, ex)
   }

  func (h *Handler) listProofs(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   rows, err := h.engine.ListProofs(c.Request.Context(), ProofState(c.Query("state")), limit)
	  if err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": rows})
  }

   func (h *Handler) listHalf(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   rows, err := h.engine.ListHalfProofs(c.Request.Context(), limit)
	   if err != nil {
		  abortBadRequest(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": rows})
   }

   func (h *Handler) listRace(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  rows, err := h.engine.ListRaceProofs(c.Request.Context(), limit)
	  if err != nil {
		  abortBadRequest(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": rows})
  }

type proofActReq struct {
	ProofID string `json:"proof_id"`
	Note    string `json:"note"`
}

  func (h *Handler) confirmProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	   var req proofActReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  p, err := h.engine.ConfirmProof(c.Request.Context(), req.ProofID, req.Note)
	  if err != nil {
		   if errors.Is(err,
			   ErrNotFound) {
			   abortBadRequest(c, err)
			   return
		  }
		  abortConflict(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, p)
   }

   func (h *Handler) dismissProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   abortForbidden(c, err)
		  return
	   }
	  var req proofActReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	   p,
		  err := h.engine.DismissProof(c.Request.Context(), req.ProofID, req.Note)
	  if err != nil {
		  abortConflict(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, p)
   }

  func (h *Handler) listAudit(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   rows, err := h.engine.ListAudit(c.Request.Context(), c.Param("borrow_id"), limit)
	  if err != nil {
		  abortBadRequest(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"audit": rows})
  }

   func (h *Handler) metrics(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, SnapshotMetrics())
  }
