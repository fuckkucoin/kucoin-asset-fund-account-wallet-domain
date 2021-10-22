// @FileName: http.go
// @Summary: HTTP 面：对外路由与请求编排。
// @Author: Talon
// @Created: 2021.10.21

package restoregate

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册灾备回灌 API 路由。
// 路由分组：
// POST /restore/plans 创建回灌计划
// GET /restore/plans 列出计划摘要
// GET /restore/plans/:plan_id 查询计划详情
// POST /restore/plans/:plan_id/freeze 推进到 freezing_writes
// POST /restore/plans/:plan_id/import 推进到 importing（含世代闸）
// POST /restore/plans/:plan_id/verify 推进到 verifying
// POST /restore/plans/:plan_id/seal 验证余额根并封印
// POST /restore/plans/:plan_id/abort 强制中止
// POST /restore/plans/:plan_id/resume-seal 续半恢复封印
// GET /restore/plans/:plan_id/gen-gates 世代闸审计记录
// GET /restore/plans/:plan_id/blocked-writes 被阻断写意图审计
// GET /restore/proofs 列出 open/clearing 脑裂证明
// GET /restore/proofs/:proof_id 查询证明详情
// POST /restore/proofs/:proof_id/approve 四眼审批
// POST /restore/proofs/:proof_id/override 运维强制解除
// GET /restore/proofs/:proof_id/export 导出证明
// POST /restore/plans/:plan_id/force-seal 脑裂解除后 ops 强制封印
   func RegisterRoutes(r gin.IRouter, svc *Service) {
	   g := r.Group("/restore")
	  g.POST("/plans", handleCreatePlan(svc))
	  g.GET("/plans", handleListPlans(svc))
	   g.GET("/plans/:plan_id", handleGetPlan(svc))
	   g.POST("/plans/:plan_id/freeze", handleFreezePlan(svc))
	  g.POST("/plans/:plan_id/import", handleStartImport(svc))
	  g.POST("/plans/:plan_id/verify", handleStartVerifying(svc))
	  g.POST("/plans/:plan_id/seal", handleVerifyAndSeal(svc))
	  g.POST("/plans/:plan_id/abort", handleAbortPlan(svc))
	   g.POST("/plans/:plan_id/resume-seal", handleResumeSeal(svc))
	  g.GET("/plans/:plan_id/gen-gates", handleGenGates(svc))
	   g.GET("/plans/:plan_id/blocked-writes", handleBlockedWrites(svc))
	  g.POST("/plans/:plan_id/force-seal", handleForceSeal(svc))
	   g.GET("/proofs", handleListProofs(svc))
	  g.GET("/proofs/:proof_id", handleGetProof(svc))
	   g.POST("/proofs/:proof_id/approve", handleApproveProof(svc))
	   g.POST("/proofs/:proof_id/override", handleOverrideProof(svc))
	  g.GET("/proofs/:proof_id/export", handleExportProof(svc))
   }

   func writeJSON(c *gin.Context, code int, v any) {
	   c.JSON(code, v)
   }

  func writeErr(c *gin.Context, err error) {
	   code := http.StatusInternalServerError
	  switch {
	   case errors.Is(err, ErrPlanNotFound), errors.Is(err, ErrSplitBrainNotFound),
		   errors.Is(err, ErrHalfSealNotFound), errors.Is(err, ErrMutexNotFound):
		   code = http.StatusNotFound
	  case errors.Is(err, ErrPlanConflict), errors.Is(err, ErrMutexConflict),
		  errors.Is(err, ErrHalfSealExists), errors.Is(err, ErrProofConflict):
		   code = http.StatusConflict
	   case errors.Is(err, ErrGenConflict), errors.Is(err, ErrGenLag),
		  errors.Is(err, ErrInvalidState), errors.Is(err, ErrEmpty),
		   errors.Is(err, ErrPlanAborted), errors.Is(err, ErrPlanSealed):
		  code = http.StatusUnprocessableEntity
	  case errors.Is(err, ErrWriteFrozen), errors.Is(err, ErrSplitBrainOpen):
		   code = http.StatusServiceUnavailable
	  case errors.Is(err, ErrChecksumMismatch):
		  code = http.StatusConflict
	   }
	   c.JSON(code, gin.H{"error": err.Error()})
   }

   func handleCreatePlan(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   var req CreatePlanRequest
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		   p, err := svc.CreatePlan(c.Request.Context(), req)
		  if err != nil {
			  writeErr(c, err)
			  return
		  }
		   Metrics.PlansCreatedTotal.Inc()
		   writeJSON(c, http.StatusCreated, p)
	  }
  }

   func handleListPlans(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   limitStr := c.DefaultQuery("limit", "20")
		   limit, _ := strconv.Atoi(limitStr)
		  plans, err := svc.ListPlans(c.Request.Context(), limit)
		  if err != nil {
			  writeErr(c, err)
			  return
		  }
		   writeJSON(c, http.StatusOK, plans)
	  }
  }

  func handleGetPlan(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  p, err := svc.GetPlan(c.Request.Context(), c.Param("plan_id"))
		  if err != nil {
			  writeErr(c, err)
			  return
		   }
		  writeJSON(c, http.StatusOK, p)
	   }
   }

  func handleFreezePlan(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  p, err := svc.FreezePlan(c.Request.Context(), c.Param("plan_id"))
		   if err != nil {
			  writeErr(c, err)
			   return
		   }
		  writeJSON(c, http.StatusOK, p)
	   }
   }

  func handleStartImport(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  var body struct {
			   LiveWatermark int64 `json:"live_watermark"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  p, err := svc.StartImport(c.Request.Context(), c.Param("plan_id"), body.LiveWatermark)
		   if err != nil {
			   writeErr(c, err)
			   return
		   }
		  writeJSON(c, http.StatusOK, p)
	  }
   }

  func handleStartVerifying(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  p, err := svc.StartVerifying(c.Request.Context(), c.Param("plan_id"))
		  if err != nil {
			  writeErr(c, err)
			  return
		   }
		  writeJSON(c, http.StatusOK, p)
	  }
  }

  func handleVerifyAndSeal(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  var body struct {
			  BalanceRootPost string `json:"balance_root_post"`
			  DiffSummary string `json:"diff_summary"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  p, err := svc.VerifyAndSeal(c.Request.Context(), c.Param("plan_id"), body.BalanceRootPost, body.DiffSummary)
		   if err != nil {
			  if errors.Is(err, ErrChecksumMismatch) {
				  Metrics.SplitBrainOpenTotal.Inc()
			  }
			   writeErr(c, err)
			  return
		  }
		   Metrics.PlansSealedTotal.Inc()
		  writeJSON(c, http.StatusOK, p)
	   }
   }

  func handleAbortPlan(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var body struct {
			   Reason string `json:"reason"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   p, err := svc.AbortPlan(c.Request.Context(), c.Param("plan_id"), body.Reason)
		   if err != nil {
			  writeErr(c, err)
			   return
		   }
		  Metrics.PlansAbortedTotal.Inc()
		  writeJSON(c, http.StatusOK, p)
	   }
  }

   func handleResumeSeal(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := svc.ResumeHalfSeal(c.Request.Context(), c.Param("plan_id")); err != nil {
			  writeErr(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"resumed": true})
	  }
  }

  func handleGenGates(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   gates, err := svc.GenGates(c.Request.Context(), c.Param("plan_id"))
		   if err != nil {
			   writeErr(c, err)
			  return
		   }
		   writeJSON(c, http.StatusOK, gates)
	  }
  }

   func handleBlockedWrites(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  limitStr := c.DefaultQuery("limit", "100")
		   limit, _ := strconv.Atoi(limitStr)
		  bws, err := svc.BlockedWrites(c.Request.Context(), c.Param("plan_id"), limit)
		   if err != nil {
			  writeErr(c, err)
			  return
		   }
		  writeJSON(c, http.StatusOK, bws)
	  }
  }

   func handleForceSeal(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   var body struct {
			   Actor string `json:"actor"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   p, err := svc.ForceSeal(c.Request.Context(), c.Param("plan_id"), strings.TrimSpace(body.Actor))
		  if err != nil {
			   writeErr(c, err)
			   return
		   }
		   Metrics.PlansSealedTotal.Inc()
		  writeJSON(c, http.StatusOK, p)
	  }
  }

  func handleListProofs(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  proofs, err := svc.ListOpenProofs(c.Request.Context())
		  if err != nil {
			  writeErr(c, err)
			   return
		  }
		   writeJSON(c, http.StatusOK, proofs)
	   }
   }

   func handleGetProof(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  proof, err := svc.ExportProof(c.Request.Context(), c.Param("proof_id"))
		   if err != nil {
			   writeErr(c, err)
			   return
		  }
		  writeJSON(c, http.StatusOK, proof)
	  }
  }

   func handleApproveProof(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  var body struct {
			  Approver string `json:"approver"`
			   Role    string `json:"role"`
			   Comment string `json:"comment"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  if err := svc.ApproveProof(c.Request.Context(), c.Param("proof_id"), body.Approver, body.Role, body.Comment); err != nil {
			  writeErr(c, err)
			  return
		  }
		  Metrics.SplitBrainClearTotal.Inc()
		  c.JSON(http.StatusOK, gin.H{"approved": true})
	  }
   }

  func handleOverrideProof(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var body struct {
			   Actor  string `json:"actor"`
			   Reason string `json:"reason"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		  if err := svc.OverrideProof(c.Request.Context(), c.Param("proof_id"), body.Actor, body.Reason); err != nil {
			  writeErr(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"overridden": true})
	   }
  }

   func handleExportProof(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  proof, err := svc.ExportProof(c.Request.Context(), c.Param("proof_id"))
		   if err != nil {
			   writeErr(c, err)
			  return
		  }
		  // 导出为 JSON 附件
		  b, _ := json.MarshalIndent(proof, "", "  ")
		  c.Header("Content-Disposition", `attachment; filename="splitbrain_proof_`+proof.ID+`.json"`)
		   c.Data(http.StatusOK, "application/json", b)
	   }
   }
