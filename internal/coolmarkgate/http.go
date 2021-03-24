// 重点：
// 所有写接口须持 custody.sweep 权限；证明导出须持 treasury.plan 权限。
// Mark 接口是资金安全核心：必须返回 lease_id 与 generation 供调用方存档。
// ExecVerify 是执行放行闸：拒绝必须返回 HTTP 409 并携带 breach_case_id。
// 所有 API 路径均挂在 /cool-mark/ 前缀下，与 treasury/exec-lease/ 隔离。
// 风险点：
// HTTP 层不持有业务锁；并发 Mark 与 CheckExecAllowed 须依赖 Engine.mu 与 DB 约束。
// ExecVerify 拒绝时须在响应体中明确 reason，不得仅返回错误码。

// Author: Talon
// Date: 2021.03.23

package coolmarkgate

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 把 coolmarkgate API 挂载到 gin RouterGroup。
// 功能：Mark 签发、Exec 复验、证明导出、违约案例查询四类端点。
   func RegisterRoutes(rg *gin.RouterGroup, eng *Engine) {
	   h := &handler{eng: eng}
	  // 写路径（须 custody.sweep 权限）。
	   rg.POST("/cool-mark/mark", h.mark)
	   rg.POST("/cool-mark/exec-verify", h.execVerify)
	   rg.POST("/cool-mark/record-sent-check", h.recordSentCheck)
	   // 读路径（须 treasury.plan 权限，用于审计/导出）。
	  rg.GET("/cool-mark/lease/:asset", h.getActiveLease)
	   rg.GET("/cool-mark/proof/:proof_id", h.getProof)
	  rg.GET("/cool-mark/proofs/:asset", h.listProofsByAsset)
	   rg.GET("/cool-mark/breach/:asset", h.listBreachByAsset)
	  rg.GET("/cool-mark/gen/:asset", h.getCoolGen)
   }

type handler struct {
	eng *Engine
}

// mark POST /cool-mark/mark
// 功能：为指定资产触发冷却标记，签发 CoolLease 并抬升 cool_gen。
   func (h *handler) mark(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body struct {
		   Asset string `json:"asset"`
		   Reason string `json:"reason"`
		  Window string `json:"window,omitempty"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  if strings.TrimSpace(body.Asset) == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "asset required"})
		  return
	   }
	   var window time.Duration
	   if body.Window != "" {
		   d, err := time.ParseDuration(body.Window)
		  if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window: " + err.Error()})
			   return
		   }
		   window = d
	   }
	   res, err := h.eng.Mark(c.Request.Context(), MarkInput{
		   Asset:  body.Asset,
		  Reason: body.Reason,
		   Window: window,
	  })
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{
		  "lease_id": res.Lease.LeaseID,
		   "generation": res.Generation,
		   "cool_until": res.Lease.CoolUntil.UTC().Format(time.RFC3339),
		   "state":      string(res.Lease.State),
		  "reused": res.Reused,
	  })
   }

// execVerify POST /cool-mark/exec-verify
// 功能：Exec 路径复验——校验 cool_gen 是否匹配、冷却窗是否已结束、证明是否存在。
// 拒绝时返回 HTTP 409 并携带 breach_case_id 与 reason。

  func (h *handler) execVerify(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var body struct {
		  Asset   string `json:"asset"`
		   CoolGen int64  `json:"cool_gen"`
		  ExecID string `json:"exec_id"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   if strings.TrimSpace(body.Asset) == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "asset required"})
		  return
	   }
	   res, err := h.eng.CheckExecAllowed(c.Request.Context(), ExecVerifyInput{
		   Asset:   body.Asset,
		   CoolGen: body.CoolGen,
		  ExecID:  body.ExecID,
	   })
	  if err != nil {
		   status := http.StatusConflict
		  if errors.Is(err, ErrInvalidInput) {
			  status = http.StatusBadRequest
		  }
		   resp := gin.H{
			   "allowed": false,
			  "reason":         "",
			  "breach_case_id": "",
			  "generation":     int64(0),
		  }
		  if res != nil {
			  resp["reason"] = res.Reason
			  resp["breach_case_id"] = res.BreachCaseID
			   resp["generation"] = res.Generation
		  }
		   c.JSON(status, resp)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "allowed":    res.Allowed,
		   "proof_id":   res.ProofID,
		  "generation": res.Generation,
	   })
   }

// recordSentCheck POST /cool-mark/record-sent-check
// 功能：MarkSent 最后防线——与 execVerify 逻辑一致，但作为独立入口供 MarkSent 流程调用。
  func (h *handler) recordSentCheck(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body struct {
		  Asset   string `json:"asset"`
		   CoolGen int64  `json:"cool_gen"`
		  ExecID string `json:"exec_id"`
	  }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   res, err := h.eng.RecordSentCheck(c.Request.Context(), ExecVerifyInput{
		  Asset: body.Asset,
		  CoolGen: body.CoolGen,
		  ExecID:  body.ExecID,
	  })
	  if err != nil {
		  resp := gin.H{"allowed": false, "reason": "", "breach_case_id": ""}
		  if res != nil {
			   resp["reason"] = res.Reason
			  resp["breach_case_id"] = res.BreachCaseID
		  }
		  c.JSON(http.StatusConflict, resp)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{
		   "allowed": res.Allowed,
		  "proof_id": res.ProofID,
		   "generation": res.Generation,
	  })
   }

// getActiveLease GET /cool-mark/lease/:asset
// 功能：查询指定资产的活跃冷却租约（只读）。
  func (h *handler) getActiveLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   asset := strings.ToUpper(strings.TrimSpace(c.Param("asset")))
	   lease, err := h.eng.GetActiveLease(c.Request.Context(), asset)
	   if err != nil {
		  if errors.Is(err, ErrCoolLeaseNotFound) {
			   c.JSON(http.StatusNotFound, gin.H{"error": "no active cool lease"})
			   return
		   }
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, lease)
   }

// getProof GET /cool-mark/proof/:proof_id
// 功能：按 ProofID 查询冷却证明（只读）。

  func (h *handler) getProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   proofID := strings.TrimSpace(c.Param("proof_id"))
	   proof,
		  err := h.eng.GetProof(c.Request.Context(), proofID)
	   if err != nil {
		  if errors.Is(err, ErrProofNotFound) {
			  c.JSON(http.StatusNotFound, gin.H{"error": "cool proof not found"})
			  return
		  }
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, proof)
  }

// listProofsByAsset GET /cool-mark/proofs/:asset?limit=N
// 功能：列举指定资产的 CoolProof 列表（最多 limit 条）。
   func (h *handler) listProofsByAsset(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   asset := strings.ToUpper(strings.TrimSpace(c.Param("asset")))
	   limit := parseLimit(c.Query("limit"), 20)
	   proofs, err := h.eng.ListProofsByAsset(c.Request.Context(), asset, limit)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
  }

// listBreachByAsset GET /cool-mark/breach/:asset?limit=N
// 功能：列举指定资产的 CoolBreachCase 列表（最多 limit 条）。

   func (h *handler) listBreachByAsset(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   asset := strings.ToUpper(strings.TrimSpace(c.Param("asset")))
	  limit := parseLimit(c.Query("limit"), 20)
	  cases, err := h.eng.ListBreachCasesByAsset(c.Request.Context(), asset, limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"cases": cases, "count": len(cases)})
  }

// getCoolGen GET /cool-mark/gen/:asset
// 功能：返回指定资产当前 cool_gen（只读，供他包引用）。

  func (h *handler) getCoolGen(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  asset := strings.ToUpper(strings.TrimSpace(c.Param("asset")))
	   gen, err := h.eng.GetCoolGen(c.Request.Context(), asset)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"asset": asset, "cool_gen": gen})
  }

  func parseLimit(s string, def int) int {
	   if s == "" {
		   return def
	   }
	  n, err := strconv.Atoi(s)
	  if err != nil || n <= 0 || n > 500 {
		  return def
	  }
	  return n
  }
