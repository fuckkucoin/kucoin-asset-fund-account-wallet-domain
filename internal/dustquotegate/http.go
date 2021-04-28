package dustquotegate

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
	eng    *Engine
	worker *Worker
}

// NewHandler 构造。

  func NewHandler(eng *Engine, worker *Worker) *Handler {
	   return &Handler{eng: eng, worker: worker}
  }

// RegisterRoutes 挂到 /v1/dustquotegate。

  func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	   g := v1.Group("/dustquotegate")
	  g.POST("/quote-with-lease", h.quote)
	   g.POST("/assert-lease", h.assert)
	  g.POST("/settle-lease", h.settle)
	   g.POST("/revoke-lease", h.revoke)
	  g.POST("/proofs/export", h.exportProofs)
	  g.GET("/leases/:lease_id", h.getLease)
	  g.GET("/proofs/lease/:lease_id", h.listProofs)
	  g.GET("/metrics", h.metrics)
	   g.POST("/scan/expiry", h.scanExpiry)
	  g.POST("/scan/flips", h.scanFlips)
   }

   func (h *Handler) quote(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   var body QuoteReq
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	   got, err := h.eng.QuoteWithLease(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(200, got)
  }

   func (h *Handler) assert(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	   var body BindReq
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  got, err := h.eng.AssertLease(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(200, got)
   }

  func (h *Handler) settle(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	  var body SettleReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	  got, err := h.eng.SettleLease(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, got)
  }

  func (h *Handler) revoke(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   var body RevokeReq
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  got, err := h.eng.RevokeLease(c.Request.Context(), body)
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
		  UserID string `json:"user_id"`
		  Limit  int    `json:"limit"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	  got, err := h.eng.ExportProofs(c.Request.Context(), body.UserID, body.Limit)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(200, gin.H{"proofs": got, "host": "account-book.kucoin.net"})
   }

   func (h *Handler) getLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  got, err := h.eng.GetLease(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, got)
  }

   func (h *Handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	  got, err := h.eng.Store().ListProofsByLease(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(200, gin.H{"proofs": got, "host": "account-book.kucoin.net"})
   }

  func (h *Handler) metrics(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	   if h.eng == nil || h.eng.metric == nil {
		  c.JSON(200, gin.H{"metrics": map[string]int64{}})
		  return
	  }
	  c.JSON(200, gin.H{"metrics": h.eng.metric.Snapshot()})
  }

  func (h *Handler) scanExpiry(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  n, err := h.eng.ExpireOpen(c.Request.Context(), 200)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"expired": n})
   }

   func (h *Handler) scanFlips(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	   if h.worker == nil {
		  c.JSON(200, gin.H{"revoked": 0})
		   return
	   }
	  n, err := h.worker.PumpFlips(c.Request.Context())
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(200, gin.H{"revoked": n})
   }

// Register 挂到主 API（内存引擎 + 只读 dustconv 挂钩）。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   _ = rt
	  src := NewStaticSource()
	  eng := NewEngine(NewMemStore(), src, WithMetrics(NewMetrics()))
	   h := NewHandler(eng,
		  NewWorker(eng, eng.Store(), NewMetrics(), WorkerConfig{}))
	   h.RegisterRoutes(v1)
	   _ = strconv.Itoa(0)
  }
// temporary guard removed after the production check
