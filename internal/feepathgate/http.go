// 路由（挂载到 /v1 下）：
// GET /feepathgate/lease/:lease_id — 查询单条租约
// GET /feepathgate/leases?biz_key=&status=&limit= — 按 biz_key/status 列表查询
// GET /feepathgate/generation?biz_key= — 查询世代状态
// GET /feepathgate/dualhit?biz_key= — 查询双贷命中
// POST /feepathgate/dualhit/:hit_id/resolve — 双人确认关闭 DualHit
// GET /feepathgate/proof/:proof_id — 查询对冲证明
// GET /feepathgate/proof/:proof_id/export — 导出（含在线验签）
// GET /feepathgate/replayblock?idem_key= — 查询幂等封堵记录
// POST /feepathgate/scan?biz_key= — 手动触发一次 biz_key 扫描
// GET /feepathgate/stats — 引擎计数器快照
// 权限：
// 查询类（GET）需 perm.ActQueryRead；
// 写入类（POST /resolve）需 perm.ActAccountMove（双人授权检查在业务层）。

// Author: KianFord
// Date: 2021.05.03

package feepathgate

import (
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 将 feepathgate 路由挂载到 v1 路由组。
// svc 可以是 nil（/未启用场景），所有接口均返回 503。
  func Register(v1 *gin.RouterGroup, svc *Service) {
	   h := &handler{svc: svc}
	  v1.GET("/feepathgate/lease/:lease_id", h.getLease)
	  v1.GET("/feepathgate/leases", h.listLeases)
	  v1.GET("/feepathgate/generation", h.getGeneration)
	  v1.GET("/feepathgate/dualhit", h.getDualHit)
	  v1.POST("/feepathgate/dualhit/:hit_id/resolve", h.resolveDualHit)
	  v1.GET("/feepathgate/proof/:proof_id", h.getProof)
	   v1.GET("/feepathgate/proof/:proof_id/export", h.exportProof)
	  v1.GET("/feepathgate/replayblock", h.getReplayBlock)
	   v1.POST("/feepathgate/scan", h.triggerScan)
	  v1.GET("/feepathgate/stats", h.getStats)
  }

type handler struct {
	svc *Service
}

   func (h *handler) checkRead(c *gin.Context) bool {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return false
	  }
	  return true
   }

   func (h *handler) checkWrite(c *gin.Context) bool {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return false
	  }
	   return true
   }

  func (h *handler) unavailable(c *gin.Context) {
	   c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrUnavailable.Error()})
  }

// GET /feepathgate/lease/:lease_id

   func (h *handler) getLease(c *gin.Context) {
	  if !h.checkRead(c) {
		   return
	  }
	   if h.svc == nil {
		   h.unavailable(c)
		   return
	  }
	   lease, err := h.svc.GetLease(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, lease)
   }

// GET /feepathgate/leases?biz_key=&status=&limit=

  func (h *handler) listLeases(c *gin.Context) {
	  if !h.checkRead(c) {
		  return
	  }
	  if h.svc == nil {
		   h.unavailable(c)
		  return
	   }
	  bizKey := c.Query("biz_key")
	   if bizKey == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "biz_key required"})
		   return
	  }
	   leases, err := h.svc.ListLeasesByBizKey(c.Request.Context(), bizKey)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   limitStr := c.DefaultQuery("limit", "50")
	   limit, _ := strconv.Atoi(limitStr)
	   if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  if len(leases) > limit {
		   leases = leases[:limit]
	  }
	   c.JSON(http.StatusOK, gin.H{
		   "biz_key": bizKey,
		  "leases": leases,
		  "count": len(leases),
	   })
  }

// GET /feepathgate/generation?biz_key=

   func (h *handler) getGeneration(c *gin.Context) {
	   if !h.checkRead(c) {
		  return
	  }
	  if h.svc == nil {
		   h.unavailable(c)
		   return
	   }
	   bizKey := c.Query("biz_key")
	   if bizKey == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "biz_key required"})
		   return
	  }
	  gen,
		  ok := h.svc.fsm.GetGeneration(bizKey)
	  if !ok {
		  c.JSON(http.StatusNotFound, gin.H{"error": "generation not found"})
		   return
	  }
	   c.JSON(http.StatusOK, gen)
  }

// GET /feepathgate/dualhit?biz_key=

  func (h *handler) getDualHit(c *gin.Context) {
	  if !h.checkRead(c) {
		   return
	   }
	  if h.svc == nil {
		   h.unavailable(c)
		   return
	  }
	   bizKey := c.Query("biz_key")
	  if bizKey == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "biz_key required"})
		  return
	  }
	  hit, err := h.svc.GetDualHit(c.Request.Context(), bizKey)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  if hit == nil {
		   c.JSON(http.StatusNotFound, gin.H{"biz_key": bizKey, "dual_hit": nil})
		  return
	  }
	   c.JSON(http.StatusOK, hit)
  }

// POST /feepathgate/dualhit/:hit_id/resolve
// Body: { "resolved_by": "ops-user@...", "note": "..." }

  func (h *handler) resolveDualHit(c *gin.Context) {
	   if !h.checkWrite(c) {
		   return
	  }
	   if h.svc == nil {
		  h.unavailable(c)
		  return
	   }
	  var body struct {
		  ResolvedBy string `json:"resolved_by" binding:"required"`
		   Note       string `json:"note"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  req := DualHitResolveReq{
		  DualHitID:  c.Param("hit_id"),
		  ResolvedBy: body.ResolvedBy,
		  Note: body.Note,
	   }
	  hit, err := h.svc.ResolveDualHit(c.Request.Context(), req)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, hit)
   }

// GET /feepathgate/proof/:proof_id
   func (h *handler) getProof(c *gin.Context) {
	   if !h.checkRead(c) {
		  return
	  }
	  if h.svc == nil {
		  h.unavailable(c)
		   return
	  }
	   proof, err := h.svc.GetHedgeProof(c.Request.Context(), c.Param("proof_id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, proof)
   }

// GET /feepathgate/proof/:proof_id/export
   func (h *handler) exportProof(c *gin.Context) {
	  if !h.checkRead(c) {
		   return
	  }
	  if h.svc == nil {
		   h.unavailable(c)
		  return
	  }
	  proof, err := h.svc.ExportProof(c.Request.Context(), c.Param("proof_id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  // Export 接口附带重新计算的 digest，供调用方自校验。
	  c.JSON(http.StatusOK, gin.H{
		  "proof":           proof,
		  "digest_verified": proof.Digest == proof.ComputeDigest(),
	   })
  }

// GET /feepathgate/replayblock?idem_key=

   func (h *handler) getReplayBlock(c *gin.Context) {
	   if !h.checkRead(c) {
		   return
	  }
	  if h.svc == nil {
		   h.unavailable(c)
		  return
	  }
	   idemKey := c.Query("idem_key")
	   if idemKey == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "idem_key required"})
		   return
	   }
	  block, err := h.svc.GetReplayBlock(c.Request.Context(), idemKey)
	  if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"idem_key": idemKey, "block": nil})
		   return
	  }
	  c.JSON(http.StatusOK, block)
   }

// POST /feepathgate/scan?biz_key=
// 手动触发一次对指定 biz_key 的双贷扫描（ops 工具）。

  func (h *handler) triggerScan(c *gin.Context) {
	  if !h.checkWrite(c) {
		  return
	   }
	   if h.svc == nil {
		   h.unavailable(c)
		  return
	   }
	   bizKey := c.Query("biz_key")
	  if bizKey == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "biz_key required"})
		  return
	   }
	   hit, err := h.svc.ScanForDualHit(c.Request.Context(), bizKey)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "biz_key":  bizKey,
		   "dual_hit": hit,
	   })
   }

// GET /feepathgate/stats
   func (h *handler) getStats(c *gin.Context) {
	   if !h.checkRead(c) {
		  return
	  }
	   if h.svc == nil {
		  h.unavailable(c)
		   return
	   }
	   c.JSON(http.StatusOK,
		   h.svc.Stats())
  }
// temporary guard removed after the production check
