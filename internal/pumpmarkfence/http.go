package pumpmarkfence

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

type handler struct {
	eng *Engine
}

// RegisterRoutes 挂载泵 Mark 闸路由。

  func RegisterRoutes(rg *gin.RouterGroup, eng *Engine) {
	  h := &handler{eng: eng}
	   rg.POST("/outbox/pump-fence/enqueue", h.enqueue)
	   rg.POST("/outbox/pump-fence/tick", h.tick)
	  rg.POST("/outbox/pump-fence/resume", h.resume)
	  rg.POST("/outbox/pump-fence/abandon", h.abandon)
	  rg.POST("/outbox/pump-fence/poison-replay", h.poison)
	   rg.POST("/outbox/pump-fence/inspect", h.inspect)
	  rg.POST("/outbox/pump-fence/expire", h.expire)
	  rg.POST("/outbox/pump-fence/export", h.export)
	  rg.POST("/outbox/pump-fence/lag-mark", h.lagMark)
	  rg.POST("/outbox/pump-fence/peer-ack", h.peerAck)
	  rg.GET("/outbox/pump-fence/gen/:deliver_effect_gen", h.getGen)
	  rg.GET("/outbox/pump-fence/by-outbox/:outbox_id", h.byOutbox)
	   rg.GET("/outbox/pump-fence/half", h.listHalf)
	  rg.GET("/outbox/pump-fence/proofs", h.listProofs)
	   rg.GET("/outbox/pump-fence/audit", h.audit)
  }

type enqueueBody struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

  func (h *handler) enqueue(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var body enqueueBody
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	   ev, err := h.eng.Enqueue(c.Request.Context(), body.Key, body.Kind, body.Payload)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, ev)
  }

type tickBody struct {
	Actor string `json:"actor"`
	Limit int    `json:"limit"`
	Sink  string `json:"sink"`
}

   func (h *handler) tick(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body tickBody
	  _ = c.ShouldBindJSON(&body)
	  sink := NewCountSink()
	  rep, err := h.eng.TickWithGenLimit(c.Request.Context(), sink.Handler(), body.Actor, body.Limit)
	  if err != nil && rep.Err == "" {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{
		  "report":             rep,
		   "handler_effects":    sink.Total(),
		   "deliver_effect_gen": firstGen(rep),
	   })
  }

  func (h *handler) resume(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body ResumeMarkReq
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  gen, err := h.eng.ResumeMark(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gen)
   }

type abandonBody struct {
	OutboxID string `json:"outbox_id"`
	Actor string `json:"actor"`
	Note  string `json:"note"`
}

  func (h *handler) abandon(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body abandonBody
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	   half, err := h.eng.AbandonHalf(c.Request.Context(), body.OutboxID, body.Actor, body.Note)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, half)
   }

   func (h *handler) poison(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body PoisonReplayReq
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	   sink := NewCountSink()
	   res, err := h.eng.PoisonReplayAssert(c.Request.Context(), body, sink.Handler())
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, res)
  }

type inspectBody struct {
	Limit int    `json:"limit"`
	Actor string `json:"actor"`
}

   func (h *handler) inspect(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body inspectBody
	  _ = c.ShouldBindJSON(&body)
	  rep, err := h.eng.InspectHalf(c.Request.Context(), body.Limit, body.Actor)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, rep)
   }

  func (h *handler) expire(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body inspectBody
	  _ = c.ShouldBindJSON(&body)
	   n, err := h.eng.ExpireStale(c.Request.Context(), body.Limit, body.Actor)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"expired": n})
  }

  func (h *handler) export(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body inspectBody
	   _ = c.ShouldBindJSON(&body)
	  bundle, err := h.eng.ExportProofs(c.Request.Context(), body.Limit, body.Actor)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, bundle)
   }

type lagMarkBody struct {
	OutboxID string `json:"outbox_id"`
	Actor string `json:"actor"`
}

   func (h *handler) lagMark(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body lagMarkBody
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   err := h.eng.MarkDeniedFromLag(c.Request.Context(), body.OutboxID, body.Actor)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"ok": false})
   }

   func (h *handler) peerAck(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body lagMarkBody
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	   half, err := h.eng.AckPeer(c.Request.Context(), body.OutboxID, body.Actor)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, half)
   }

  func (h *handler) getGen(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   gen, err := h.eng.GetGen(c.Request.Context(), c.Param("deliver_effect_gen"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gen)
   }

   func (h *handler) byOutbox(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   gen, err := h.eng.GetGenByOutbox(c.Request.Context(), c.Param("outbox_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gen)
  }

  func (h *handler) listHalf(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   limit, _ := strconv.Atoi(c.Query("limit"))
	   rows, err := h.eng.ListHalfOpen(c.Request.Context(), limit)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, rows)
  }

   func (h *handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  limit, _ := strconv.Atoi(c.Query("limit"))
	  rows, err := h.eng.ListProofs(c.Request.Context(), c.Query("outbox_id"), limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, rows)
   }

  func (h *handler) audit(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  limit, _ := strconv.Atoi(c.Query("limit"))
	  rows, err := h.eng.ListAudit(c.Request.Context(), strings.TrimSpace(c.Query("outbox_id")), limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, rows)
  }

  func firstGen(rep TickReport) string {
	   if len(rep.GenIDs) == 0 {
		  return ""
	   }
	   return rep.GenIDs[0]
  }
