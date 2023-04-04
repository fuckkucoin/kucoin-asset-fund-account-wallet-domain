// 功能：occupbucketfence HTTP API——HoldWithFence、ReleaseAssert、ReclassAssert、证明导出、巡检触发。
// - 写接口需 ActOpsAdmin；读接口 ActQueryRead。
// - 错误走 mw.WriteProblem。
// 不变量：
// - 时间 UTC RFC3339。
// - 证明导出幂等。

// 作者：Cason
// 日期：2023.03.06

package occupbucketfence

import (
	"errors"
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 处理器。

type Handler struct {
	engine *Engine
	store  Store
	metrics *Metrics
	worker  *Worker
}

// NewHandler 构造。

   func NewHandler(engine *Engine, store Store, metrics *Metrics, worker *Worker) *Handler {
	  return &Handler{engine: engine, store: store, metrics: metrics, worker: worker}
  }

// RegisterRoutes 注册 /occup-bucket-fence。

  func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	  g := v1.Group("/occup-bucket-fence")
	   g.POST("/hold-with-fence", h.handleHold)
	  g.POST("/release-assert", h.handleRelease)
	   g.POST("/reclass-assert", h.handleReclass)
	   g.POST("/issue-fence", h.handleIssue)
	  g.POST("/half/resume", h.handleResume)
	  g.POST("/proofs/export", h.handleExport)
	   g.POST("/proofs/prove", h.handleProve)
	  g.POST("/proofs/close", h.handleClose)
	  g.GET("/fences/:fence_id", h.handleGetFence)
	  g.GET("/lots/:lot_id", h.handleGetLot)
	  g.GET("/proofs/:proof_id", h.handleGetProof)
	   g.POST("/scan/patrol", h.handlePatrol)
	  g.POST("/scan/expire", h.handleExpire)
  }

  func (h *Handler) requireOps(c *gin.Context) bool {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return false
	   }
	   return true
   }

   func (h *Handler) requireRead(c *gin.Context) bool {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return false
	   }
	  return true
   }

  func (h *Handler) handleHold(c *gin.Context) {
	  if !h.requireOps(c) {
		  return
	   }
	  var req HoldWithFenceReq
	  if !mw.BindJSON(c, &req) {
		  return
	   }
	  resp, err := h.engine.HoldWithFence(c.Request.Context(), req)
	   if err != nil {
		  if h.metrics != nil {
			   h.metrics.RecordHold(false)
		   }
		   if errors.Is(err, ErrHalfOpen) && resp != nil {
			  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
			   return
		  }
		  mw.WriteProblem(c, err)
		  return
	  }
	  if h.metrics != nil {
		   h.metrics.RecordHold(true)
	  }
	   c.JSON(http.StatusOK, resp)
   }

  func (h *Handler) handleRelease(c *gin.Context) {
	  if !h.requireOps(c) {
		   return
	  }
	   var req ReleaseAssertReq
	  if !mw.BindJSON(c, &req) {
		   return
	   }
	  resp, err := h.engine.ReleaseAssert(c.Request.Context(), req)
	   h.writeMutate(c, "release", resp, err)
   }

  func (h *Handler) handleReclass(c *gin.Context) {
	   if !h.requireOps(c) {
		   return
	  }
	   var req ReclassAssertReq
	   if !mw.BindJSON(c, &req) {
		   return
	   }
	  resp, err := h.engine.ReclassAssert(c.Request.Context(), req)
	  h.writeMutate(c, "reclass", resp, err)
  }

  func (h *Handler) writeMutate(c *gin.Context, op string, resp *MutateResp, err error) {
	  if err != nil {
		   if h.metrics != nil {
			  if errors.Is(err, ErrReclassMutex) {
				   if op == "release" {
					   h.metrics.RecordRelease("mutex")
				  } else {
					   h.metrics.RecordReclass("mutex")
				  }
			   } else {
				   if op == "release" {
					  h.metrics.RecordRelease("err")
				   } else {
					   h.metrics.RecordReclass("err")
				   }
			  }
		   }
		  if resp != nil && resp.Proof != nil {
			  if h.metrics != nil {
				  h.metrics.RecordBreak()
			  }
			  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
			   return
		  }
		   mw.WriteProblem(c, err)
		  return
	  }
	  if h.metrics != nil {
		   if op == "release" {
			  h.metrics.RecordRelease("ok")
		   } else {
			  h.metrics.RecordReclass("ok")
		  }
	   }
	   c.JSON(http.StatusOK, resp)
   }

  func (h *Handler) handleIssue(c *gin.Context) {
	  if !h.requireOps(c) {
		  return
	   }
	  var req IssueFenceReq
	   if !mw.BindJSON(c, &req) {
		   return
	  }
	  f, err := h.engine.IssueFence(c.Request.Context(), req)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, f)
  }

   func (h *Handler) handleResume(c *gin.Context) {
	   if !h.requireOps(c) {
		  return
	  }
	  var req struct {
		  LotID  string `json:"lot_id"`
		   CaseID string `json:"case_id"`
	   }
	   if !mw.BindJSON(c, &req) {
		   return
	   }
	  half, err := h.engine.ResumeHalf(c.Request.Context(), req.LotID, req.CaseID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, half)
  }

  func (h *Handler) handleExport(c *gin.Context) {
	  if !h.requireRead(c) {
		   return
	   }
	  var req ExportProofReq
	  if !mw.BindJSON(c, &req) {
		  return
	   }
	  list,
		  err := h.engine.ExportProofs(c.Request.Context(), req.LotID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": list})
   }

  func (h *Handler) handleProve(c *gin.Context) {
	   if !h.requireOps(c) {
		  return
	  }
	  var req struct {
		  ProofID string `json:"proof_id"`
	  }
	  if !mw.BindJSON(c, &req) {
		  return
	   }
	  p,
		  err := h.engine.ProveBreak(c.Request.Context(), req.ProofID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, p)
  }

   func (h *Handler) handleClose(c *gin.Context) {
	  if !h.requireOps(c) {
		   return
	   }
	  var req struct {
		  ProofID string `json:"proof_id"`
	  }
	   if !mw.BindJSON(c, &req) {
		   return
	   }
	  p, err := h.engine.CloseBreak(c.Request.Context(), req.ProofID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, p)
  }

   func (h *Handler) handleGetFence(c *gin.Context) {
	  if !h.requireRead(c) {
		   return
	   }
	  f, err := h.engine.GetFence(c.Request.Context(), c.Param("fence_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, f)
  }

  func (h *Handler) handleGetLot(c *gin.Context) {
	   if !h.requireRead(c) {
		  return
	   }
	   sn, err := h.engine.Book().SnapshotLot(c.Request.Context(), c.Param("lot_id"), true)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, sn)
   }

   func (h *Handler) handleGetProof(c *gin.Context) {
	  if !h.requireRead(c) {
		  return
	  }
	  p, err := h.store.GetProof(c.Request.Context(), c.Param("proof_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, p)
   }

  func (h *Handler) handlePatrol(c *gin.Context) {
	   if !h.requireOps(c) {
		  return
	  }
	  list, err := h.engine.PatrolConserve(c.Request.Context(), 200)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": list})
   }

  func (h *Handler) handleExpire(c *gin.Context) {
	   if !h.requireOps(c) {
		  return
	  }
	  n, err := h.engine.ExpireStale(c.Request.Context(), 200)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"expired": n})
  }
