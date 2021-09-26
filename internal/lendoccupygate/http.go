package lendoccupygate

import (
	"errors"
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handler lendoccupygate HTTP 处理器集合。
type Handler struct {
	engine  *Engine
	scanner *Scanner
	store   Store
	metrics *Metrics
}

// NewHandler 构造 Handler。

  func NewHandler(engine *Engine, scanner *Scanner, store Store, metrics *Metrics) *Handler {
	  return &Handler{
		  engine: engine,
		   scanner: scanner,
		  store:   store,
		  metrics: metrics,
	  }
   }

// RegisterRoutes 注册所有 lendoccupygate 路由到 v1 路由组。
  func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	   g := v1.Group("/lendoccupygate")
	   // 挂单占用
	  g.POST("/place-bound", h.handlePlaceBound)
	   g.POST("/settle", h.handleSettle)
	  g.POST("/cancel", h.handleCancel)
	   // 证明导出
	  g.POST("/proofs/export", h.handleExportProofs)
	   g.GET("/proofs/dup/:proof_id", h.handleGetDupProof)
	  g.GET("/proofs/residue/:proof_id", h.handleGetResidueProof)
	   // 租约查询
	  g.GET("/leases/:lease_id", h.handleGetLease)
	   g.GET("/leases/order/:order_id", h.handleGetLeaseByOrder)
	  g.GET("/leases/user/:user_id", h.handleListLeasesByUser)
	  // 扫描触发（ops 主动触发，Worker 自动周期触发）
	  g.POST("/scan/half-place", h.handleScanHalfPlace)
  }

// ── 挂单占用 ──────────────────────────────────────────────────────────────────

  func (h *Handler) handlePlaceBound(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var req PlaceBoundReq
	  if !mw.BindJSON(c, &req) {
		  return
	  }
	   resp, err := h.engine.PlaceBound(c.Request.Context(), req)
	   if err != nil {
		  if h.metrics != nil {
			   h.metrics.RecordPlaceBound(false)
		  }
		   mw.WriteProblem(c, err)
		  return
	   }
	   if h.metrics != nil {
		   h.metrics.RecordPlaceBound(true)
	   }
	   c.JSON(http.StatusOK, resp)
  }

  func (h *Handler) handleSettle(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var req SettleReq
	   if !mw.BindJSON(c, &req) {
		  return
	   }
	   resp, err := h.engine.Settle(c.Request.Context(), req)
	  if err != nil {
		   if errors.Is(err, ErrGenConsumed) {
			  if h.metrics != nil {
				  h.metrics.RecordSettle("dup")
			   }
			   // 重复消费：返回 409 + dup proof
			   c.JSON(http.StatusConflict, gin.H{
				  "error":     err.Error(),
				  "dup_proof": resp.DupProof,
				  "lease":     resp.Lease,
			  })
			  return
		   }
		  if h.metrics != nil {
			  h.metrics.RecordSettle("err")
		  }
		   mw.WriteProblem(c, err)
		  return
	   }
	  if h.metrics != nil {
		   h.metrics.RecordSettle("ok")
	   }
	   c.JSON(http.StatusOK, resp)
  }

   func (h *Handler) handleCancel(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var req CancelReq
	  if !mw.BindJSON(c, &req) {
		   return
	  }
	   resp,
		   err := h.engine.Cancel(c.Request.Context(), req)
	  if err != nil {
		   if errors.Is(err, ErrGenConsumed) {
			   if h.metrics != nil {
				  h.metrics.RecordCancel("dup")
			  }
			  c.JSON(http.StatusConflict, gin.H{
				  "error": err.Error(),
				   "dup_proof": resp.DupProof,
				  "lease":     resp.Lease,
			  })
			   return
		   }
		   if h.metrics != nil {
			   h.metrics.RecordCancel("err")
		   }
		  mw.WriteProblem(c, err)
		   return
	  }
	   if h.metrics != nil {
		  h.metrics.RecordCancel("ok")
	   }
	  c.JSON(http.StatusOK, resp)
   }

// ── 证明导出 ──────────────────────────────────────────────────────────────────

  func (h *Handler) handleExportProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var req ExportProofReq
	  if !mw.BindJSON(c, &req) {
		   return
	   }
	  resp,
		  err := h.engine.ExportProofs(c.Request.Context(), req)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, resp)
   }

  func (h *Handler) handleGetDupProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  proofID := c.Param("proof_id")
	   proof, err := h.store.GetDupProof(c.Request.Context(), proofID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, proof)
   }

  func (h *Handler) handleGetResidueProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  proofID := c.Param("proof_id")
	   proof, err := h.store.GetResidueProof(c.Request.Context(), proofID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, proof)
   }

// ── 租约查询 ──────────────────────────────────────────────────────────────────

  func (h *Handler) handleGetLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  leaseID := c.Param("lease_id")
	  lease,
		   err := h.store.GetLease(c.Request.Context(), leaseID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, lease)
   }

  func (h *Handler) handleGetLeaseByOrder(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   orderID := c.Param("order_id")
	   lease, err := h.store.GetLeaseByOrder(c.Request.Context(), orderID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, lease)
  }

  func (h *Handler) handleListLeasesByUser(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   userID := c.Param("user_id")
	   limit := 50
	  if ls := c.Query("limit"); ls != "" {
		   if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			   limit = n
		   }
	  }
	  leases, err := h.store.ListLeasesByUser(c.Request.Context(), userID, limit)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"leases": leases, "count": len(leases)})
   }

// ── 扫描触发 ──────────────────────────────────────────────────────────────────

   func (h *Handler) handleScanHalfPlace(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   result, err := h.scanner.ScanHalfPlace(c.Request.Context())
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, result)
   }
