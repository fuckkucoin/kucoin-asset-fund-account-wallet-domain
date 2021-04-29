package feebumpgate

import (
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 集合。

type Handler struct {
	eng *Engine
	worker *Worker
	metric *Metrics
}

// NewHandler 构造。
   func NewHandler(eng *Engine, worker *Worker) *Handler {
	  return &Handler{eng: eng, worker: worker}
   }

// WithMetrics 挂指标。
   func (h *Handler) WithMetrics(m *Metrics) *Handler {
	   if h != nil {
		  h.metric = m
	  }
	   return h
  }

// RegisterRoutes 挂到 /v1/feebumpgate。
  func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	  if h == nil || v1 == nil {
		  return
	  }
	   g := v1.Group("/feebumpgate")
	  g.POST("/sign-with-gen", h.sign)
	   g.POST("/assert-occupancy", h.assertOcc)
	  g.POST("/broadcast", h.broadcast)
	   g.POST("/finalize", h.finalize)
	   g.POST("/force-retire", h.forceRetire)
	  g.POST("/proofs/export", h.exportProofs)
	   g.GET("/gens/:gen_id", h.getGen)
	  g.GET("/transfers/:transfer_id/gens", h.listGens)
	   g.GET("/orphans", h.listOrphans)
	   g.GET("/metrics", h.metrics)
	  g.POST("/scan/orphans", h.scanOrphans)
	  g.POST("/scan/expired", h.scanExpired)
   }

   func (h *Handler) sign(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	  var body SignWithGenReq
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   if body.Actor == "" {
		   body.Actor = perm.FromRequest(c.Request, "").ID
	  }
	   got,
		  err := h.eng.SignFeeBumpWithGen(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, got)
   }

  func (h *Handler) assertOcc(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	  var body AssertOccReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	  got, err := h.eng.AssertOccupancy(c.Request.Context(), body)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, got)
  }

  func (h *Handler) broadcast(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  var body BroadcastReq
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	  got, err := h.eng.BroadcastReplace(c.Request.Context(), body)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(200, got)
   }

   func (h *Handler) finalize(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		  GenID string `json:"gen_id"`
		  Actor string `json:"actor"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  got, err := h.eng.Finalize(c.Request.Context(), body.GenID, body.Actor)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, got)
  }

   func (h *Handler) forceRetire(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	   var body RetireReq
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	  body.Force = true
	  got, err := h.eng.ForceRetire(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(200, got)
  }

   func (h *Handler) exportProofs(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	  var body struct {
		  TransferID string `json:"transfer_id"`
	  }
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	  got, err := h.eng.ExportProofs(c.Request.Context(), body.TransferID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, gin.H{"proofs": got})
   }

  func (h *Handler) getGen(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	  got, err := h.eng.GetGen(c.Request.Context(), c.Param("gen_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(200, got)
  }

  func (h *Handler) listGens(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   got, err := h.eng.ListGens(c.Request.Context(), c.Param("transfer_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, gin.H{"gens": got})
  }

   func (h *Handler) listOrphans(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   got, err := h.eng.ListOpenOrphans(c.Request.Context(), limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, gin.H{"orphans": got})
  }

   func (h *Handler) metrics(c *gin.Context) {
	   if h.metric == nil {
		  c.JSON(200, gin.H{})
		  return
	   }
	  c.JSON(200, h.metric.Snapshot())
  }

  func (h *Handler) scanOrphans(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	   if h.worker == nil {
		   c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker unavailable"})
		   return
	  }
	   n,
		   err := h.worker.ScanOrphans(c.Request.Context())
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(200, gin.H{"detected": n})
  }

   func (h *Handler) scanExpired(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	   if h.worker == nil {
		  c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker unavailable"})
		  return
	   }
	   n, err := h.worker.ScanExpired(c.Request.Context())
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, gin.H{"retired": n})
  }

// Register 挂到主 API（内存引擎）。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   _ = rt
	  metric := NewMetrics()
	   eng := NewEngine(NewMemStore(),
		   WithMetrics(metric))
	   w := NewWorker(eng, eng.Store(), metric, WorkerConfig{})
	  h := NewHandler(eng, w).WithMetrics(metric)
	   h.RegisterRoutes(v1)
  }
