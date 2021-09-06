// 功能：occseggate HTTP / Ops API。
// 端点：
// POST /occseggate/before-freeze — BeforeFreezeWithGen
// POST /occseggate/commit — CommitAssertGen
// POST /occseggate/abort — AbortAssertGen
// POST /occseggate/expire-revoke — ExpireRevokeGens
// POST /occseggate/reopen-admit — ReopenAdmit
// POST /occseggate/refuse-live-unfreeze — RefuseLiveUnfreeze
// GET /occseggate/seg/:id
// GET /occseggate/seg/grant/:grant_idem
// GET /occseggate/segs/:ticket_id
// GET /occseggate/proof/:id
// GET /occseggate/proof/:id/export
// GET /occseggate/proofs
// POST /occseggate/proof/ack
// GET /occseggate/revocation/:id
// GET /occseggate/audit/:ticket_id
// GET /occseggate/metrics

// 作者：Cason
// 日期：2021.09.05

package occseggate

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
	  rg.POST("/occseggate/before-freeze", h.beforeFreeze)
	  rg.POST("/occseggate/commit", h.commit)
	   rg.POST("/occseggate/abort", h.abort)
	  rg.POST("/occseggate/expire-revoke", h.expireRevoke)
	  rg.POST("/occseggate/reopen-admit", h.reopenAdmit)
	   rg.POST("/occseggate/refuse-live-unfreeze", h.refuseLive)
	   rg.GET("/occseggate/seg/:id", h.getSeg)
	  rg.GET("/occseggate/seg/grant/:grant_idem", h.getSegGrant)
	   rg.GET("/occseggate/segs/:ticket_id", h.listSegs)
	  rg.GET("/occseggate/proof/:id", h.getProof)
	  rg.GET("/occseggate/proof/:id/export", h.exportProof)
	   rg.GET("/occseggate/proofs", h.listProofs)
	   rg.POST("/occseggate/proof/ack", h.ackProof)
	  rg.GET("/occseggate/revocation/:id", h.getRev)
	   rg.GET("/occseggate/audit/:ticket_id", h.listAudit)
	  rg.GET("/occseggate/metrics", h.metrics)
   }

  func (h *Handler) mustPerm(c *gin.Context) bool {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   abortForbidden(c, err)
		   return false
	   }
	   return true
  }

type occupyReq struct {
	TicketID string `json:"ticket_id"`
	GrantIdem string `json:"grant_idem"`
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
}

   func (h *Handler) beforeFreeze(c *gin.Context) {
	  if !h.mustPerm(c) {
		  return
	   }
	  var req occupyReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		  return
	  }
	   res,
		   err := h.engine.BeforeFreezeWithGen(c.Request.Context(), OccOccupyRequest(req))
	   if err != nil {
		   c.JSON(http.StatusConflict, gin.H{"allowed": false, "error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"allowed": true, "ticket": res.Ticket, "grant": res.Grant, "seg": ToSegExport(res.Seg)})
   }

type genReq struct {
	GrantIdem string `json:"grant_idem"`
	OccSegGen *uint64 `json:"occ_seg_gen"`
	Actor     string  `json:"actor"`
}

  func (h *Handler) writeGate(c *gin.Context, res *GateResult, err error) {
	   if err != nil {
		   body := gin.H{"allowed": false, "error": err.Error()}
		  if res != nil {
			   body["reason"] = res.RejectReason
			  if res.Proof != nil {
				   body["proof"] = ToExport(res.Proof)
			   }
			  if res.Seg != nil {
				  body["seg"] = ToSegExport(res.Seg)
			   }
		   }
		   c.JSON(http.StatusConflict, body)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"allowed": true, "seg": ToSegExport(res.Seg), "grant": res.Grant})
   }

   func (h *Handler) commit(c *gin.Context) {
	  if !h.mustPerm(c) {
		   return
	  }
	  var req genReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		  return
	  }
	  res, err := h.engine.CommitAssertGen(c.Request.Context(), GenRequest{GrantIdem: req.GrantIdem, OccSegGen: req.OccSegGen, Actor: req.Actor})
	   h.writeGate(c, res, err)
   }

  func (h *Handler) abort(c *gin.Context) {
	   if !h.mustPerm(c) {
		  return
	  }
	   var req genReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		  return
	   }
	   res, err := h.engine.AbortAssertGen(c.Request.Context(), GenRequest{GrantIdem: req.GrantIdem, OccSegGen: req.OccSegGen, Actor: req.Actor})
	  h.writeGate(c, res, err)
  }

type expireReq struct {
	TicketID string `json:"ticket_id"`
	Note string `json:"note"`
}

   func (h *Handler) expireRevoke(c *gin.Context) {
	  if !h.mustPerm(c) {
		   return
	  }
	   var req expireReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	   rev,
		   err := h.engine.ExpireRevokeGens(c.Request.Context(), req.TicketID, req.Note)
	  if err != nil {
		   abortConflict(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, rev)
  }

   func (h *Handler) reopenAdmit(c *gin.Context) {
	  if !h.mustPerm(c) {
		  return
	   }
	   var req expireReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  t, err := h.engine.ReopenAdmit(c.Request.Context(), req.TicketID)
	   if err != nil {
		   abortConflict(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, t)
  }

type liveReq struct {
	GrantIdem string `json:"grant_idem"`
}

  func (h *Handler) refuseLive(c *gin.Context) {
	  if !h.mustPerm(c) {
		  return
	  }
	   var req liveReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	   p, err := h.engine.RefuseLiveUnfreeze(c.Request.Context(), req.GrantIdem)
	  c.JSON(http.StatusConflict, gin.H{"allowed": false, "error": err.Error(), "proof": ToExport(p)})
   }

   func (h *Handler) getSeg(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	   }
	   s, err := h.engine.GetSeg(c.Request.Context(), c.Param("id"))
	  if err != nil {
		   if errors.Is(err, ErrNotFound) {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			   return
		  }
		   abortConflict(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, ToSegExport(s))
   }

  func (h *Handler) getSegGrant(c *gin.Context) {
	   if !h.mustPerm(c) {
		  return
	   }
	  s, err := h.engine.GetSegByGrant(c.Request.Context(), c.Param("grant_idem"))
	   if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, ToSegExport(s))
  }

   func (h *Handler) listSegs(c *gin.Context) {
	   if !h.mustPerm(c) {
		  return
	  }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  list, err := h.engine.ListSegs(c.Request.Context(), c.Param("ticket_id"), limit)
	  if err != nil {
		  abortConflict(c, err)
		  return
	  }
	  out := make([]SegExport,
		  0, len(list))
	  for _, s := range list {
		  out = append(out, ToSegExport(s))
	  }
	  c.JSON(http.StatusOK, out)
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
	  c.JSON(http.StatusOK, ToExport(p))
  }

  func (h *Handler) exportProof(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	  }
	  p, err := h.engine.GetProof(c.Request.Context(), c.Param("id"))
	  if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	  }
	   b, err := MarshalProof(p)
	  if err != nil {
		  abortConflict(c, err)
		  return
	   }
	   c.Data(http.StatusOK, "application/json", b)
  }

  func (h *Handler) listProofs(c *gin.Context) {
	  if !h.mustPerm(c) {
		  return
	  }
	  st := ProofState(c.DefaultQuery("state", string(ProofOpen)))
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   list, err := h.engine.ListProofs(c.Request.Context(), st, limit)
	  if err != nil {
		   abortConflict(c, err)
		  return
	  }
	  out := make([]ProofExport, 0, len(list))
	  for _, p := range list {
		  out = append(out, ToExport(p))
	   }
	   c.JSON(http.StatusOK, out)
   }

type ackReq struct {
	ID   string `json:"id"`
	Note string `json:"note"`
}

  func (h *Handler) ackProof(c *gin.Context) {
	  if !h.mustPerm(c) {
		   return
	  }
	  var req ackReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		  return
	  }
	  p, err := h.engine.AckProof(c.Request.Context(), req.ID, req.Note)
	   if err != nil {
		   abortConflict(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, ToExport(p))
  }

  func (h *Handler) getRev(c *gin.Context) {
	  if !h.mustPerm(c) {
		   return
	   }
	   r, err := h.engine.GetRevocation(c.Request.Context(), c.Param("id"))
	  if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, r)
   }

   func (h *Handler) listAudit(c *gin.Context) {
	  if !h.mustPerm(c) {
		  return
	   }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   list, err := h.engine.ListAudit(c.Request.Context(), c.Param("ticket_id"), limit)
	  if err != nil {
		   abortConflict(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, list)
  }

   func (h *Handler) metrics(c *gin.Context) {
	   if !h.mustPerm(c) {
		   return
	  }
	  c.JSON(http.StatusOK, SnapshotMetrics())
   }
