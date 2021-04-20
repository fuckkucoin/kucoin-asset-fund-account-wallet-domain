// HTTP/ops API — 粉尘泄漏守卫运营接口。
// 端点：
// GET /dustleak/summary 运营面板概要
// GET /dustleak/leaks LeakCase 列表（按状态过滤）
// GET /dustleak/leaks/:id LeakCase 详情
// POST /dustleak/leaks/:id/transition 状态跳转（detected→rolling_back 等）
// POST /dustleak/leaks/:id/rollback 创建回滚腿（需双人 dualReqID）
// POST /dustleak/rollbacks/:id/execute 执行回滚（账务逆记）
// POST /dustleak/rollbacks/:id/confirm 确认 Conserve（关闭 LeakCase）
// POST /dustleak/rollbacks/:id/fail 回滚失败，进入 dispute
// GET /dustleak/rollbacks/:id/proof 导出回滚证明
// POST /dustleak/gates/block 运营强制挡兑（需 dualReqID）
// POST /dustleak/gates/release 运营强制解挡（需 dualReqID）
// GET /dustleak/gates/:user_id/:asset 查询挡兑闸状态
// POST /dustleak/scan 运营触发即时扫描（需 ActOpsAdmin）
// GET /dustleak/policy_gens/:asset 查询策略世代列表
// GET /dustleak/snapshots/:snap_key 查询牌价快照

// Author: Cason
// Date: 2021.04.19

package dustleak

import (
	"strconv"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 将 dustleak HTTP 端点挂载到 v1 RouterGroup。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service, detector *Detector) {
	   _ = rt
	   h := &handler{svc: svc,
		  detector: detector}
	   dl := v1.Group("/dustleak")
	   {
		  dl.GET("/summary", h.summary)
		   dl.GET("/leaks", h.listLeaks)
		  dl.GET("/leaks/:id", h.getLeakCase)
		   dl.POST("/leaks/:id/transition", h.transitionLeak)
		   dl.POST("/leaks/:id/rollback", h.postRollback)
		   dl.POST("/rollbacks/:id/execute", h.executeRollback)
		   dl.POST("/rollbacks/:id/confirm", h.confirmRollback)
		   dl.POST("/rollbacks/:id/fail", h.failRollback)
		  dl.GET("/rollbacks/:id/proof", h.rollbackProof)
		  dl.POST("/gates/block", h.forceBlock)
		  dl.POST("/gates/release", h.forceRelease)
		  dl.GET("/gates/:user_id/:asset", h.gateStatus)
		  dl.POST("/scan", h.triggerScan)
		   dl.GET("/policy_gens/:asset", h.policyGens)
		  dl.GET("/snapshots/:snap_key", h.snapshotByKey)
	  }
  }

type handler struct {
	svc *Service
	detector *Detector
}

// ── 概要 ─────────────────────────────────────────────────────────────────────

  func (h *handler) summary(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(200, h.svc.Summary())
   }

// ── LeakCase ─────────────────────────────────────────────────────────────────

  func (h *handler) listLeaks(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	  statusFilter := LeakStatus(c.Query("status"))
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   cases := h.svc.AllLeakCases(statusFilter,
		  limit)
	  c.JSON(200, gin.H{"cases": cases, "count": len(cases)})
   }

  func (h *handler) getLeakCase(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	  lc, err := h.svc.GetLeakCase(c.Param("id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(200, lc)
  }

type transitionReq struct {
	From LeakStatus `json:"from"`
	To    LeakStatus `json:"to"`
	Actor string     `json:"actor"`
}

  func (h *handler) transitionLeak(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	   var req transitionReq
	  if !mw.BindJSON(c, &req) {
		   return
	  }
	   if req.Actor == "" {
		   req.Actor = perm.FromRequest(c.Request, "ops-dustleak").ID
	  }
	   lc, err := h.svc.TransitionLeak(c.Param("id"), req.From, req.To, req.Actor)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(200, lc)
   }

// ── Rollback ─────────────────────────────────────────────────────────────────

type postRollbackReq struct {
	ExecutedBy string `json:"executed_by"`
	DualReqID  string `json:"dual_req_id"` // 双人审批凭证；必填
}

  func (h *handler) postRollback(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	  var req postRollbackReq
	  if !mw.BindJSON(c, &req) {
		  return
	  }
	  if req.ExecutedBy == "" {
		  req.ExecutedBy = perm.FromRequest(c.Request, "ops-dustleak").ID
	  }
	   rb, err := h.svc.PostRollback(c.Param("id"), req.ExecutedBy, req.DualReqID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(201, rb)
  }

type executeRollbackReq struct {
	Actor string `json:"actor"`
}

  func (h *handler) executeRollback(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	   var req executeRollbackReq
	   if !mw.BindJSON(c, &req) {
		  return
	  }
	  if req.Actor == "" {
		   req.Actor = perm.FromRequest(c.Request, "ops-dustleak").ID
	  }
	   rb, err := h.svc.ExecuteRollback(c.Param("id"), req.Actor)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(200, rb)
   }

type confirmRollbackReq struct {
	ProofHash string `json:"proof_hash"`
	Actor string `json:"actor"`
}

   func (h *handler) confirmRollback(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  var req confirmRollbackReq
	  if !mw.BindJSON(c, &req) {
		   return
	  }
	   if req.Actor == "" {
		  req.Actor = perm.FromRequest(c.Request, "ops-dustleak").ID
	   }
	  rb, err := h.svc.ConfirmRollback(c.Param("id"), req.ProofHash, req.Actor)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(200, rb)
  }

type failRollbackReq struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

   func (h *handler) failRollback(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	  var req failRollbackReq
	   if !mw.BindJSON(c, &req) {
		   return
	  }
	   if req.Actor == "" {
		   req.Actor = perm.FromRequest(c.Request, "ops-dustleak").ID
	  }
	   rb, err := h.svc.FailRollback(c.Param("id"), req.Actor, req.Reason)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, rb)
  }

  func (h *handler) rollbackProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  proof, err := h.svc.RollbackProofOf(c.Param("id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, proof)
  }

// ── ConvertGate ───────────────────────────────────────────────────────────────

type gateReq struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	LeakID    string `json:"leak_id"`
	Actor     string `json:"actor"`
	DualReqID string `json:"dual_req_id"`
}

  func (h *handler) forceBlock(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  return
	   }
	  var req gateReq
	   if !mw.BindJSON(c, &req) {
		   return
	   }
	   if req.Actor == "" {
		  req.Actor = perm.FromRequest(c.Request, "ops-dustleak").ID
	   }
	   gate, err := h.svc.ForceBlock(req.UserID, req.Asset, req.LeakID, req.Actor, req.DualReqID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(200, gate)
   }

  func (h *handler) forceRelease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   var req gateReq
	  if !mw.BindJSON(c, &req) {
		   return
	   }
	   if req.Actor == "" {
		   req.Actor = perm.FromRequest(c.Request, "ops-dustleak").ID
	  }
	  if err := h.svc.ForceRelease(req.UserID, req.Asset, req.Actor, req.DualReqID); err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(200, gin.H{"status": "released"})
  }

   func (h *handler) gateStatus(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	  userID := c.Param("user_id")
	  asset := c.Param("asset")
	   status := h.svc.GateStatusOf(userID, asset)
	   c.JSON(200, gin.H{
		   "user_id": userID,
		   "asset":   asset,
		  "status": status,
		   "at": time.Now().UTC(),
	   })
   }

// ── 扫描触发 ──────────────────────────────────────────────────────────────────

type scanReq struct {
	Records []ConvertRecord `json:"records"`
	Source string `json:"source"`
}

  func (h *handler) triggerScan(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		   return
	  }
	  var req scanReq
	  if !mw.BindJSON(c, &req) {
		  return
	  }
	  if req.Source == "" {
		  req.Source = DetectorSourceAPI
	  }
	   result := h.detector.ScanConverts(req.Records, req.Source)
	   c.JSON(200, result)
   }

// ── 策略世代 / 快照查询 ────────────────────────────────────────────────────────

  func (h *handler) policyGens(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return
	   }
	  asset := c.Param("asset")
	  latest, err := h.svc.LatestPolicyGen(asset)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, latest)
   }

  func (h *handler) snapshotByKey(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   snap, err := h.svc.SnapshotByKey(c.Param("snap_key"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(200, snap)
  }
