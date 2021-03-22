// @FileName: http.go
// @Summary: cerdecidegate 包内 http 实现。
// 做什么：DecideWithLease / AssertLease / 证明导出 / 运维吊销 HTTP。
// 写接口需 ActAccountMove 或 ActOpsAdmin；读接口 ActQueryRead。
// 不变量：错误走 mw.WriteProblem；不改既有 Host 后缀。
// @Author: KianFord
// @Created: 2021.03.11

package cerdecidegate

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
	metric *Metrics
}

// NewHandler 构造。

   func NewHandler(eng *Engine, worker *Worker) *Handler {
	  return &Handler{eng: eng,
		  worker: worker}
  }

// WithMetrics 挂指标。

   func (h *Handler) WithMetrics(m *Metrics) *Handler {
	  if h != nil {
		  h.metric = m
	   }
	  return h
  }

// RegisterRoutes 挂到 /v1/cerdecidegate。
   func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	  if h == nil || v1 == nil {
		   return
	   }
	  g := v1.Group("/cerdecidegate")
	   g.POST("/decide-with-lease", h.decide)
	  g.POST("/assert-lease", h.assert)
	   g.POST("/consume-lease", h.consume)
	   g.POST("/revoke-lease", h.revoke)
	   g.POST("/proofs/export", h.exportProofs)
	  g.GET("/leases/:lease_id", h.getLease)
	  g.GET("/proofs/lease/:lease_id", h.listProofs)
	  g.GET("/metrics", h.metrics)
	   g.POST("/scan/expiry", h.scanExpiry)
	  g.POST("/scan/flips", h.scanFlips)
	  g.POST("/scan/risks", h.scanRisks)
	   g.POST("/flip/policy", h.flipPolicy)
	   g.POST("/flip/risk", h.flipRisk)
  }

  func (h *Handler) decide(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  var body DecideReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   got, err := h.eng.DecideWithLease(c.Request.Context(), body)
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

   func (h *Handler) consume(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   var body ConsumeReq
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  got, err := h.eng.ConsumeLease(c.Request.Context(), body)
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
		  BatchID string `json:"batch_id"`
		  Limit int `json:"limit"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	   got, err := h.eng.ExportProofs(c.Request.Context(), body.BatchID, body.Limit)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(200, gin.H{"proofs": got})
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
	  got, err := h.eng.ListProofs(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(200, gin.H{"proofs": got})
  }

   func (h *Handler) metrics(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   if h.metric == nil {
		  c.JSON(http.StatusOK, gin.H{})
		  return
	  }
	  c.JSON(http.StatusOK, h.metric.Snapshot())
   }

  func (h *Handler) scanExpiry(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	   if h.worker == nil {
		  c.JSON(200, gin.H{"expired": 0})
		   return
	  }
	  n, err := h.worker.ScanExpiry(c.Request.Context())
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

  func (h *Handler) scanRisks(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	  if h.worker == nil {
		   c.JSON(200, gin.H{"revoked": 0})
		  return
	   }
	  n, err := h.worker.PumpRisks(c.Request.Context())
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(200, gin.H{"revoked": n})
  }

   func (h *Handler) flipPolicy(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	  var body FlipEvent
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  n, err := h.eng.OnFlip(c.Request.Context(), body)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(200, gin.H{"revoked": n})
   }

  func (h *Handler) flipRisk(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		   BatchID string `json:"batch_id"`
		  OldRisk string `json:"old_dest_risk"`
		   NewRisk string `json:"new_dest_risk"`
		  PolicyID string `json:"policy_id"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   n, err := h.eng.RaiseDestRisk(c.Request.Context(), body.BatchID, body.OldRisk, body.NewRisk, body.PolicyID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, gin.H{"revoked": n})
   }

// Register 挂到主 API（内存引擎）。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   _ = rt
	   src := NewMemorySource()
	   metric := NewMetrics()
	   eng := NewEngine(NewMemStore(), src, WithMetrics(metric))
	   w := NewWorker(eng,
		   eng.Store(), metric, WorkerConfig{})
	   h := NewHandler(eng, w).WithMetrics(metric)
	  h.RegisterRoutes(v1)
	   _ = strconv.Itoa(0)
  }
