package clrcreditgate

import (
	"errors"
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handler clrcreditgate HTTP 处理器集合。
type Handler struct {
	engine *Engine
	worker *Worker
	store   Store
	metrics *Metrics
}

// NewHandler 构造 Handler。

   func NewHandler(engine *Engine, worker *Worker, store Store, metrics *Metrics) *Handler {
	   return &Handler{
		  engine: engine,
		  worker: worker,
		  store: store,
		  metrics: metrics,
	  }
   }

// RegisterRoutes 注册所有 clrcreditgate 路由到 v1 路由组。
   func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	  g := v1.Group("/clrcreditgate")
	  // 释放并签发 Lease
	   g.POST("/release-with-lease", h.handleReleaseWithLease)
	   // 消费 Lease 并贷记
	  g.POST("/apply-with-lease", h.handleApplyWithLease)
	  // 关批守卫
	   g.POST("/close-batch-guard", h.handleCloseBatchGuard)
	  // 显式废弃 Lease（ops）
	   g.POST("/void-lease", h.handleVoidLease)
	  // 证明导出
	   g.POST("/proofs/export", h.handleExportProofs)
	   g.GET("/proofs/:proof_id", h.handleGetProof)
	   g.GET("/proofs/item/:item_id", h.handleListProofsByItem)
	   g.GET("/proofs/batch/:batch_id", h.handleListProofsByBatch)
	  // Lease 查询
	   g.GET("/leases/:lease_id", h.handleGetLease)
	   g.GET("/leases/item/:item_id", h.handleGetLeaseByItem)
	  g.GET("/leases/batch/:batch_id", h.handleListLeasesByBatch)
	  // 批次排空状态
	   g.GET("/drain/:batch_id", h.handleGetBatchDrain)
	  // 手动触发扫描（ops 接口）
	  g.POST("/scan/expiry", h.handleScanExpiry)
	   g.POST("/scan/unredeemed", h.handleScanUnredeemed)
	  g.POST("/scan/drain", h.handleScanDrain)
   }

// ── 写接口 ────────────────────────────────────────────────────────────────────

  func (h *Handler) handleReleaseWithLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body struct {
		  ItemID string `json:"item_id"`
		   Actor string `json:"actor"`
		   Epoch int64  `json:"epoch"`
		   Token string `json:"token"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   res, err := h.engine.ReleaseWithLease(c.Request.Context(), body.ItemID, body.Actor, body.Epoch, body.Token)
	  if err != nil {
		  h.metrics.incRelease("err")
		  mw.WriteProblem(c, err)
		  return
	   }
	  h.metrics.incRelease("ok")
	   h.metrics.incProofSaved("lease_issued")
	   c.JSON(http.StatusOK, gin.H{
		   "item_id":  res.ItemID,
		  "batch_id": res.BatchID,
		   "lease":    res.Lease,
	  })
  }

   func (h *Handler) handleApplyWithLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body struct {
		  ItemID  string `json:"item_id"`
		   LeaseID string `json:"lease_id"`
		   Actor string `json:"actor"`
		   FenceEpoch int64 `json:"fence_epoch"`
		  FenceToken string `json:"fence_token"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	   fence := Fence{Epoch: body.FenceEpoch, Token: body.FenceToken}
	   res, err := h.engine.ApplyWithLease(c.Request.Context(), body.ItemID, body.LeaseID, body.Actor, fence)
	   if err != nil {
		  kind := "err"
		  status := http.StatusInternalServerError
		   switch {
		   case errors.Is(err, ErrDupApply), errors.Is(err, ErrLeaseConsumed):
			   kind = "dup"
			   status = http.StatusConflict
		   case errors.Is(err, ErrLeaseExpired), errors.Is(err, ErrLeaseVoid):
			  kind = "expired"
			   status = http.StatusGone
		   case errors.Is(err, ErrLeaseFenceStale):
			  kind = "stale"
			   status = http.StatusConflict
		  }
		  h.metrics.incApply(kind)
		   c.JSON(status, gin.H{"error": err.Error(), "proof": res.Proof})
		   return
	   }
	  h.metrics.incApply("ok")
	  if res.Proof != nil {
		  h.metrics.incProofSaved(string(res.Proof.Kind))
	  }
	  c.JSON(http.StatusOK, gin.H{
		  "item_id": res.ItemID,
		   "lease_id": res.LeaseID,
		  "proof": res.Proof,
		   "position_after": res.PositionAfter,
	   })
   }

  func (h *Handler) handleCloseBatchGuard(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body struct {
		   BatchID string `json:"batch_id"`
		   Actor string `json:"actor"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   res, err := h.engine.CloseBatchGuard(c.Request.Context(), body.BatchID, body.Actor)
	   if err != nil && !errors.Is(err, ErrDrainBlocked) {
		   h.metrics.incCloseBatchGuard("err")
		  mw.WriteProblem(c, err)
		  return
	  }
	  if res != nil && res.Allowed {
		   h.metrics.incCloseBatchGuard("allowed")
		   c.JSON(http.StatusOK, res)
		  return
	  }
	  h.metrics.incCloseBatchGuard("blocked")
	  c.JSON(http.StatusConflict, res)
   }

  func (h *Handler) handleVoidLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body struct {
		   LeaseID string `json:"lease_id"`
		  Actor   string `json:"actor"`
		   Reason string `json:"reason"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	   if err := h.engine.VoidLease(c.Request.Context(), body.LeaseID, body.Actor, body.Reason); err != nil {
		  h.metrics.incLeaseVoidedOps()
		  mw.WriteProblem(c, err)
		  return
	   }
	  h.metrics.incLeaseVoidedOps()
	  c.JSON(http.StatusOK, gin.H{"voided": true, "lease_id": body.LeaseID})
  }

// ── 读接口 ────────────────────────────────────────────────────────────────────

   func (h *Handler) handleExportProofs(c *gin.Context) {
	  var body struct {
		  BatchID string `json:"batch_id"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  proofs, err := h.engine.ExportProofs(c.Request.Context(), body.BatchID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
   }

  func (h *Handler) handleGetProof(c *gin.Context) {
	  proofID := c.Param("proof_id")
	   p, err := h.store.GetProof(c.Request.Context(), proofID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, p)
   }

  func (h *Handler) handleListProofsByItem(c *gin.Context) {
	   itemID := c.Param("item_id")
	   limit := parseLimit(c.Query("limit"), 50)
	   proofs, err := h.store.ListProofs(c.Request.Context(), ProofListFilter{
		   ItemID: itemID,
		  Limit:  limit,
	   })
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
   }

   func (h *Handler) handleListProofsByBatch(c *gin.Context) {
	   batchID := c.Param("batch_id")
	   limit := parseLimit(c.Query("limit"), 200)
	   proofs, err := h.store.ListProofs(c.Request.Context(), ProofListFilter{
		  BatchID: batchID,
		   Limit:   limit,
	  })
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
  }

   func (h *Handler) handleGetLease(c *gin.Context) {
	   leaseID := c.Param("lease_id")
	   l, err := h.store.GetLease(c.Request.Context(), leaseID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, l)
   }

  func (h *Handler) handleGetLeaseByItem(c *gin.Context) {
	  itemID := c.Param("item_id")
	   l, err := h.store.GetLeaseByItem(c.Request.Context(), itemID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, l)
   }

  func (h *Handler) handleListLeasesByBatch(c *gin.Context) {
	  batchID := c.Param("batch_id")
	   limit := parseLimit(c.Query("limit"), 200)
	   leases, err := h.store.ListLeases(c.Request.Context(), LeaseListFilter{
		   BatchID: batchID,
		  Limit: limit,
	   })
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"leases": leases, "count": len(leases)})
   }

   func (h *Handler) handleGetBatchDrain(c *gin.Context) {
	  batchID := c.Param("batch_id")
	   d,
		   err := h.store.GetBatchDrain(c.Request.Context(), batchID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, d)
  }

// ── 扫描触发 ──────────────────────────────────────────────────────────────────

  func (h *Handler) handleScanExpiry(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  if h.worker == nil {
		  c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker not available"})
		  return
	  }
	  res := h.worker.ScanExpiredLeases(c.Request.Context())
	  c.JSON(http.StatusOK, res)
   }

  func (h *Handler) handleScanUnredeemed(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  if h.worker == nil {
		   c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker not available"})
		  return
	  }
	   res := h.worker.ScanUnredeemed(c.Request.Context())
	   c.JSON(http.StatusOK, res)
  }

  func (h *Handler) handleScanDrain(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   if h.worker == nil {
		  c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker not available"})
		   return
	  }
	  res := h.worker.PumpDrain(c.Request.Context())
	  c.JSON(http.StatusOK, res)
  }

// ── 辅助 ──────────────────────────────────────────────────────────────────────

   func parseLimit(s string, def int) int {
	  if s == "" {
		   return def
	  }
	   v, err := strconv.Atoi(s)
	  if err != nil || v <= 0 {
		   return def
	   }
	  if v > 1000 {
		  return 1000
	   }
	  return v
   }
