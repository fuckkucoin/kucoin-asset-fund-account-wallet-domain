package haltepochgate

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
)

// Handler ops API。
type Handler struct {
	svc *Service
}

  func NewHandler(svc *Service) *Handler {
	   if svc == nil {
		   svc = New()
	  }
	  return &Handler{svc: svc}
  }

  func (h *Handler) Register(g *gin.RouterGroup) {
	   if g == nil || h == nil {
		  return
	  }
	  g.GET("/halt-epoch-gate/healthz", h.health)
	  g.GET("/halt-epoch-gate/epoch", h.getEpoch)
	  g.GET("/halt-epoch-gate/epochs", h.listEpochs)
	  g.POST("/halt-epoch-gate/engage", h.engage)
	  g.POST("/halt-epoch-gate/clear", h.clear)
	   g.POST("/halt-epoch-gate/vote", h.vote)
	   g.POST("/halt-epoch-gate/seal-resume", h.sealResume)
	   g.POST("/halt-epoch-gate/lane-assert", h.laneAssert)
	  g.POST("/halt-epoch-gate/broadcast", h.broadcast)
	   g.POST("/halt-epoch-gate/credit", h.credit)
	  g.GET("/halt-epoch-gate/lane", h.lane)
	  g.GET("/halt-epoch-gate/proofs", h.proofs)
	  g.GET("/halt-epoch-gate/proofs/export", h.exportProofs)
	  g.POST("/halt-epoch-gate/proofs/:id/prove", h.prove)
	  g.POST("/halt-epoch-gate/proofs/:id/close", h.closeProof)
	   g.GET("/halt-epoch-gate/cases/:id", h.getCase)
	   g.GET("/halt-epoch-gate/audit", h.audit)
	   g.GET("/halt-epoch-gate/metrics", h.metrics)
  }

   func (h *Handler) health(c *gin.Context) {
	   c.JSON(http.StatusOK, gin.H{"ok": true, "module": "haltepochgate"})
   }

   func (h *Handler) getEpoch(c *gin.Context) {
	   ep, err := h.svc.CurrentEpoch(c.Request.Context(), c.Query("scope"), c.Query("chain"), c.Query("asset"))
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"epoch": ep, "lane": ep.LaneConclusion(), "digest": EpochDigest(*ep)})
  }

   func (h *Handler) listEpochs(c *gin.Context) {
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   list, err := h.svc.ListEpochs(c.Request.Context(), limit)
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"epochs": list})
   }

   func (h *Handler) engage(c *gin.Context) {
	   var body struct {
		   Scope string `json:"scope"`
		  Chain  string `json:"chain"`
		  Asset  string `json:"asset"`
		  Actor  string `json:"actor"`
		  Reason string `json:"reason"`
		   IdemKey string `json:"idem_key"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		  return
	  }
	   ep, err := h.svc.EngageWithEpoch(c.Request.Context(), EngageRequest{
		   Scope: body.Scope, Chain: body.Chain, Asset: body.Asset,
		  Actor: body.Actor, Reason: body.Reason, IdemKey: body.IdemKey,
	  })
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		  return
	  }
	  ObserveEngage()
	  c.JSON(http.StatusOK, gin.H{"epoch": ep, "action": "engage", "lane": ep.LaneConclusion()})
   }

   func (h *Handler) clear(c *gin.Context) {
	  var body struct {
		   Scope string `json:"scope"`
		   Chain string `json:"chain"`
		  Asset string `json:"asset"`
		   Actor string `json:"actor"`
		   Reason  string `json:"reason"`
		   IdemKey string `json:"idem_key"`
		   Force   bool   `json:"force"`
		  FromBridge bool `json:"from_bridge"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		  return
	   }
	   ep, cs, err := h.svc.ClearWithEpoch(c.Request.Context(), ClearRequest{
		   Scope: body.Scope, Chain: body.Chain, Asset: body.Asset,
		   Actor: body.Actor, Reason: body.Reason, IdemKey: body.IdemKey,
		   Force:      body.Force,
		   FromBridge: body.FromBridge,
	  })
	  if err != nil {
		  c.JSON(statusOf(err), gin.H{"error": err.Error()})
		   return
	  }
	   ObserveClear()
	   c.JSON(http.StatusOK, gin.H{"epoch": ep, "case": cs, "action": "clear", "lane": ep.LaneConclusion()})
  }

   func (h *Handler) vote(c *gin.Context) {
	   var body struct {
		   EpochID string `json:"epoch_id"`
		   Actor   string `json:"actor"`
		  Reason  string `json:"reason"`
		   Approve bool   `json:"approve"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		   return
	  }
	  q, err := h.svc.VoteClear(c.Request.Context(), body.EpochID, body.Actor, body.Reason, body.Approve)
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"quorum": q})
   }

   func (h *Handler) sealResume(c *gin.Context) {
	   var body struct {
		   Scope string `json:"scope"`
		   Chain string `json:"chain"`
		   Asset string `json:"asset"`
		  Actor string `json:"actor"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		  return
	   }
	   ep, err := h.svc.SealResume(c.Request.Context(), body.Scope, body.Chain, body.Asset, body.Actor)
	   if err != nil {
		  c.JSON(statusOf(err), gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"epoch": ep, "action": "seal_resume"})
   }

  func (h *Handler) laneAssert(c *gin.Context) {
	   var body struct {
		  Scope string `json:"scope"`
		   Chain string `json:"chain"`
		  Asset string `json:"asset"`
		   Lane  string `json:"lane"`
		   CarriedGen int64 `json:"carried_gen"`
		   Account string `json:"account"`
		   Amount    string `json:"amount"`
		   IdemKey   string `json:"idem_key"`
		   AllowTick string `json:"allow_ticket"`
		  IntentID  string `json:"intent_id"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		  return
	  }
	   in, conc, err := h.svc.LaneAssert(c.Request.Context(), LaneAssertRequest{
		   Scope: body.Scope, Chain: body.Chain, Asset: body.Asset,
		   Lane: FundLane(strings.TrimSpace(body.Lane)), CarriedGen: body.CarriedGen,
		   Account: body.Account, Amount: body.Amount, IdemKey: body.IdemKey,
		   AllowTick: body.AllowTick, IntentID: body.IntentID,
	   })
	   if err != nil {
		   if conc != nil && conc.ProofID != "" {
			   ObserveSlip()
			  ObserveLaneReject()
		  }
		   c.JSON(statusOf(err), gin.H{"error": err.Error(), "lane": conc, "intent": in})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"intent": in, "lane": conc})
   }

   func (h *Handler) broadcast(c *gin.Context) {
	  var body struct {
		  IntentID string `json:"intent_id"`
		  CarriedGen int64  `json:"carried_gen"`
		  Actor      string `json:"actor"`
	  }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		  return
	  }
	  in, err := h.svc.AssertBroadcast(c.Request.Context(), BroadcastRequest(body))
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error(), "intent": in})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"intent": in})
   }

  func (h *Handler) credit(c *gin.Context) {
	  var body struct {
		   IntentID string `json:"intent_id"`
		  CarriedGen int64  `json:"carried_gen"`
		  Actor      string `json:"actor"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		   return
	  }
	  in, err := h.svc.AssertCredit(c.Request.Context(), CreditRequest(body))
	  if err != nil {
		  c.JSON(statusOf(err), gin.H{"error": err.Error(), "intent": in})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"intent": in})
   }

   func (h *Handler) lane(c *gin.Context) {
	  lane := FundLane(strings.TrimSpace(c.DefaultQuery("lane", "deposit")))
	  c.JSON(http.StatusOK, h.svc.LaneOf(c.Request.Context(), lane, c.Query("scope"), c.Query("chain"), c.Query("asset")))
   }

   func (h *Handler) proofs(c *gin.Context) {
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   st := SlipState(c.Query("state"))
	  list, err := h.svc.ListProofs(c.Request.Context(), st, limit)
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": list})
  }

  func (h *Handler) exportProofs(c *gin.Context) {
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	   ex,
		  err := h.svc.ExportProofs(c.Request.Context(), limit)
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, ex)
  }

   func (h *Handler) prove(c *gin.Context) {
	   p, err := h.svc.ProveSlip(c.Request.Context(), c.Param("id"))
	  if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"proof": p})
   }

  func (h *Handler) closeProof(c *gin.Context) {
	   p, err := h.svc.CloseSlip(c.Request.Context(), c.Param("id"))
	   if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proof": p})
  }

  func (h *Handler) getCase(c *gin.Context) {
	   cs, err := h.svc.GetCase(c.Request.Context(), c.Param("id"))
	  if err != nil {
		   c.JSON(statusOf(err), gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"case": cs})
   }

   func (h *Handler) audit(c *gin.Context) {
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  list, err := h.svc.RecentAudit(c.Request.Context(), limit)
	  if err != nil {
		  c.JSON(statusOf(err), gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"events": list})
  }

  func (h *Handler) metrics(c *gin.Context) {
	  c.JSON(http.StatusOK, Metrics())
  }

  func statusOf(err error) int {
	  if err == nil {
		  return http.StatusOK
	   }
	  switch err {
	   case ErrActorRequired, ErrReasonRequired, ErrLaneUnknown, ErrScopeInvalid, ErrEpochMissing, ErrEpochStale:
		   return http.StatusBadRequest
	  case ErrQuorumDenied, ErrBridgeClear, ErrLaneSealed, ErrBroadcastDenied, ErrCreditDenied, ErrIntentRevoked, ErrRaceMutex, ErrClearInProgress, ErrEngageInProgress:
		  return http.StatusConflict
	   case ErrEpochNotFound, ErrIntentNotFound, ErrProofNotFound, ErrCaseNotFound:
		   return http.StatusNotFound
	  case ErrReadGateFailed, ErrStoreUnavailable:
		   return http.StatusServiceUnavailable
	   default:
		  return http.StatusBadRequest
	  }
  }
