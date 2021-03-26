package closelatecredit

import (
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
}

// NewHandler 构造。

   func NewHandler(engine *Engine) *Handler { return &Handler{engine: engine} }

// Register 路由。
  func (h *Handler) Register(rg *gin.RouterGroup) {
	   rg.POST("/closelatecredit/ensure", h.ensure)
	   rg.POST("/closelatecredit/close", h.close)
	  rg.POST("/closelatecredit/reopen", h.reopen)
	   rg.POST("/closelatecredit/confirm-open", h.confirm)
	  rg.POST("/closelatecredit/credit-assert", h.creditAssert)
	  rg.POST("/closelatecredit/assert-observe", h.assertObserve)
	   rg.POST("/closelatecredit/assert-outbox", h.assertOutbox)
	   rg.POST("/closelatecredit/assert-fiat", h.assertFiat)
	  rg.POST("/closelatecredit/dispute-resolve", h.resolve)
	  rg.GET("/closelatecredit/epoch/:user_id", h.getEpoch)
	  rg.GET("/closelatecredit/proof/:id", h.getProof)
	  rg.GET("/closelatecredit/proof/:id/export", h.exportProof)
	   rg.GET("/closelatecredit/proofs", h.listProofs)
	   rg.GET("/closelatecredit/hold/:id", h.getHold)
	   rg.GET("/closelatecredit/audit/:user_id", h.listAudit)
	   rg.GET("/closelatecredit/metrics", h.metrics)
  }

   func (h *Handler) mustPerm(c *gin.Context) bool {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   abortForbidden(c, err)
		   return false
	  }
	  return true
  }

type closeBody struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Reason string `json:"reason"`
	Actor  string `json:"actor"`
	IdemKey string `json:"idem_key"`
}

  func (h *Handler) ensure(c *gin.Context) {
	   if !h.mustPerm(c) {
		  return
	   }
	   var req closeBody
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	  ep, err := h.engine.EnsureOpen(c.Request.Context(), req.UserID, req.Book, req.Actor)
	  if err != nil {
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"epoch": ToEpochExport(ep)})
  }

   func (h *Handler) close(c *gin.Context) {
	  if !h.mustPerm(c) {
		  return
	  }
	   var req closeBody
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  ep, err := h.engine.CloseWithEpoch(c.Request.Context(), CloseRequest{
		   UserID: req.UserID, Book: req.Book, Reason: req.Reason, Actor: req.Actor, IdemKey: req.IdemKey,
	   })
	   if err != nil {
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"epoch": ToEpochExport(ep)})
  }

  func (h *Handler) reopen(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	   }
	  var req closeBody
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  ep, err := h.engine.ReopenWithEpoch(c.Request.Context(), ReopenRequest{
		   UserID: req.UserID,
		  Book:   req.Book, Reason: req.Reason, Actor: req.Actor, IdemKey: req.IdemKey,
	  })
	  if err != nil {
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"epoch": ToEpochExport(ep)})
  }

   func (h *Handler) confirm(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	   }
	   var req closeBody
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	  }
	   ep, err := h.engine.ConfirmOpen(c.Request.Context(), req.UserID, req.Book, req.Actor)
	  if err != nil {
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"epoch": ToEpochExport(ep)})
  }

type assertBody struct {
	UserID      string `json:"user_id"`
	ClaimedUser string `json:"claimed_user"`
	Book string `json:"book"`
	Source string `json:"source"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	TxRef        string  `json:"tx_ref"`
	ClaimedEpoch *uint64 `json:"close_epoch"`
	IdemKey      string  `json:"idem_key"`
	Actor        string  `json:"actor"`
	ForceLive bool `json:"force_live"`
	Note string `json:"note"`
}

  func (h *Handler) writeGate(c *gin.Context, res *GateResult, err error) {
	  if err != nil {
		  body := gin.H{"allowed": false, "error": err.Error()}
		  if res != nil {
			   body["decision"] = res.Decision
			   body["reason"] = res.RejectReason
			  if res.Proof != nil {
				   body["proof"] = ToExport(res.Proof)
			   }
			  if res.Hold != nil {
				   body["hold"] = ToHoldExport(res.Hold)
			  }
			   if res.Epoch != nil {
				   body["epoch"] = ToEpochExport(res.Epoch)
			   }
		   }
		   c.JSON(http.StatusConflict, body)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"allowed": true, "decision": res.Decision, "epoch": ToEpochExport(res.Epoch)})
  }

   func (h *Handler) creditAssert(c *gin.Context) {
	  if !h.mustPerm(c) {
		   return
	  }
	  var req assertBody
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  res, err := h.engine.CreditAssert(c.Request.Context(), CreditAssertRequest{
		   UserID: req.UserID, ClaimedUser: req.ClaimedUser, Book: req.Book,
		  Source: ParseCreditSource(req.Source),
		  Asset:  req.Asset, Amount: req.Amount,
		   TxRef: req.TxRef, ClaimedEpoch: req.ClaimedEpoch, IdemKey: req.IdemKey,
		  Actor: req.Actor, ForceLive: req.ForceLive, Note: req.Note,
	  })
	  h.writeGate(c, res, err)
   }

   func (h *Handler) assertObserve(c *gin.Context) {
	  h.assertSource(c, SourceObserve)
   }

  func (h *Handler) assertOutbox(c *gin.Context) {
	  h.assertSource(c, SourceOutbox)
  }

   func (h *Handler) assertFiat(c *gin.Context) {
	   h.assertSource(c, SourceFiatCallback)
   }

  func (h *Handler) assertSource(c *gin.Context, src CreditSource) {
	  if !h.mustPerm(c) {
		   return
	   }
	  var req assertBody
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	  res, err := h.engine.CreditAssert(c.Request.Context(), CreditAssertRequest{
		  UserID: req.UserID,
		   Book:   req.Book, Source: src, Asset: req.Asset,
		   Amount: req.Amount, TxRef: req.TxRef, ClaimedEpoch: req.ClaimedEpoch,
		   Actor: req.Actor,
	  })
	   h.writeGate(c, res, err)
   }

type resolveBody struct {
	HoldID string `json:"hold_id"`
	ProofID string `json:"proof_id"`
	Actor   string `json:"actor"`
	Note    string `json:"note"`
	Reject bool `json:"reject"`
	ExportOnly bool `json:"export_only"`
}

  func (h *Handler) resolve(c *gin.Context) {
	   if !h.mustPerm(c) {
		  return
	   }
	   var req resolveBody
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	   res, err := h.engine.DisputeResolve(c.Request.Context(), DisputeResolveRequest{
		   HoldID: req.HoldID, ProofID: req.ProofID, Actor: req.Actor,
		   Note: req.Note, Reject: req.Reject, ExportOnly: req.ExportOnly,
	  })
	   if err != nil {
		  c.JSON(http.StatusConflict, gin.H{"allowed": false, "error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "allowed": false, "decision": res.Decision,
		   "proof": ToExport(res.Proof), "hold": ToHoldExport(res.Hold),
	  })
  }

  func (h *Handler) getEpoch(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	  }
	  book := c.Query("book")
	   ep, err := h.engine.CurrentEpoch(c.Request.Context(), c.Param("user_id"), book)
	   if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"epoch": ToEpochExport(ep)})
  }

  func (h *Handler) getProof(c *gin.Context) {
	   if !h.mustPerm(c) {
		  return
	  }
	   p, err := h.engine.GetProof(c.Request.Context(), c.Param("id"))
	   if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"proof": ToExport(p)})
  }

   func (h *Handler) exportProof(c *gin.Context) {
	  if !h.mustPerm(c) {
		  return
	   }
	  ex, err := h.engine.ExportProof(c.Request.Context(), c.Param("id"))
	  if err != nil {
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"export": ex})
   }

  func (h *Handler) listProofs(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	   }
	  limit, _ := strconv.Atoi(c.Query("limit"))
	   rows, err := h.engine.ListProofs(c.Request.Context(), c.Query("user_id"), limit)
	  if err != nil {
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		   return
	  }
	   out := make([]ProofExport, 0, len(rows))
	  for _, p := range rows {
		   out = append(out, ToExport(p))
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": out})
   }

  func (h *Handler) getHold(c *gin.Context) {
	  if !h.mustPerm(c) {
		   return
	   }
	   hold, err := h.engine.GetHold(c.Request.Context(), c.Param("id"))
	  if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"hold": ToHoldExport(hold)})
  }

  func (h *Handler) listAudit(c *gin.Context) {
	  if !h.mustPerm(c) {
		   return
	  }
	  limit, _ := strconv.Atoi(c.Query("limit"))
	   rows, err := h.engine.ListAudit(c.Request.Context(), c.Param("user_id"), limit)
	  if err != nil {
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"audit": rows})
  }

   func (h *Handler) metrics(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	  }
	   c.JSON(http.StatusOK, SnapshotMetrics())
   }
