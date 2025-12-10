// 端点：
// GET /isodebt/v1/pairs/:userID/:symbol/generation — 查询当前世代
// GET /isodebt/v1/pairs/:userID/:symbol/generations — 历史世代列表
// POST /isodebt/v1/leases — 签发比率租约
// GET /isodebt/v1/leases/:leaseID — 查询租约
// POST /isodebt/v1/leases/:leaseID/validate — 校验租约
// POST /isodebt/v1/gates — 转入裁决
// GET /isodebt/v1/gates/:gateID — 查询裁决
// GET /isodebt/v1/drifts/:userID/:symbol — 列出 open 漂移案例
// GET /isodebt/v1/drifts/case/:caseID — 查询漂移案例
// POST /isodebt/v1/drifts/:caseID/proof — 提交漂移证明
// POST /isodebt/v1/drifts/:caseID/close — 四眼确认关闭
// GET /isodebt/v1/proofs/:proofID — 导出证明
// GET /isodebt/v1/ops/debit-locks — 活跃 Debit 锁（ops）

// Author: Cason
// Date: 2021.08.01

package isodebt

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// RegisterRoutes 在 gin.RouterGroup 下注册 isodebt 路由。
   func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	   v1 := rg.Group("/isodebt/v1")
	  // 世代
	   v1.GET("/pairs/:userID/:symbol/generation", handleGetGeneration(svc))
	  v1.GET("/pairs/:userID/:symbol/generations", handleListGenerations(svc))
	  // 租约
	  v1.POST("/leases", handleIssueRatioLease(svc))
	  v1.GET("/leases/:leaseID", handleGetLease(svc))
	   v1.POST("/leases/:leaseID/validate", handleValidateLease(svc))
	   // 转入裁决
	  v1.POST("/gates", handleAdjudicateTransferIn(svc))
	   v1.GET("/gates/:gateID", handleGetGate(svc))
	   // 漂移
	   v1.GET("/drifts/:userID/:symbol", handleListOpenDrifts(svc))
	   v1.GET("/drifts/case/:caseID", handleGetDriftCase(svc))
	   v1.POST("/drifts/:caseID/proof", handleSubmitProof(svc))
	  v1.POST("/drifts/:caseID/close", handleConfirmClose(svc))
	   // 证明导出
	   v1.GET("/proofs/:proofID", handleExportProof(svc))
	  // ops
	  v1.GET("/ops/debit-locks", handleOpsDebitLocks(svc))
  }

// ─────────────────────────────────────────────
// 世代处理器
// ─────────────────────────────────────────────

  func handleGetGeneration(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   userID := strings.TrimSpace(c.Param("userID"))
		   symbol := strings.ToUpper(strings.TrimSpace(c.Param("symbol")))
		  gen, err := svc.CurrentGeneration(c.Request.Context(), userID, symbol)
		  if err != nil {
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gen)
	  }
  }

  func handleListGenerations(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  userID := strings.TrimSpace(c.Param("userID"))
		   symbol := strings.ToUpper(strings.TrimSpace(c.Param("symbol")))
		   limit := 20
		  if l := c.Query("limit"); l != "" {
			  if v, err := strconv.Atoi(l); err == nil && v > 0 {
				  limit = v
			  }
		  }
		   gens, err := svc.ListGenerations(c.Request.Context(), userID, symbol, limit)
		   if err != nil {
			  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"items": gens, "count": len(gens)})
	   }
   }

// ─────────────────────────────────────────────
// 租约处理器
// ─────────────────────────────────────────────

type issueLeaseReq struct {
	UserID string `json:"user_id" binding:"required"`
	Symbol string `json:"symbol"  binding:"required"`
	IssuedBy string `json:"issued_by"`
}

   func handleIssueRatioLease(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var req issueLeaseReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   lease, err := svc.IssueRatioLease(c.Request.Context(), req.UserID, req.Symbol, req.IssuedBy)
		   if err != nil {
			  status := http.StatusBadRequest
			  if isOverLimitErr(err) {
				  status = http.StatusForbidden
			   }
			  c.JSON(status, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusCreated, lease)
	   }
  }

  func handleGetLease(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  leaseID := strings.TrimSpace(c.Param("leaseID"))
		   lease, err := svc.GetLease(c.Request.Context(), leaseID)
		   if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(http.StatusOK, lease)
	  }
   }

   func handleValidateLease(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   leaseID := strings.TrimSpace(c.Param("leaseID"))
		  lease, err := svc.ValidateLease(c.Request.Context(), leaseID)
		  if err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "lease": lease})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"valid": true, "lease": lease})
	  }
   }

// ─────────────────────────────────────────────
// 转入裁决处理器
// ─────────────────────────────────────────────

type adjudicateReq struct {
	LeaseID string `json:"lease_id"    binding:"required"`
	UserID string `json:"user_id"     binding:"required"`
	Symbol   string `json:"symbol"      binding:"required"`
	Currency string `json:"currency"`
	Amount      string `json:"amount"      binding:"required"`
	RequestedBy string `json:"requested_by"`
	TransferID  string `json:"transfer_id"`
}

   func handleAdjudicateTransferIn(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   var req adjudicateReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		   amt, err := decimal.NewFromString(req.Amount)
		   if err != nil || !amt.IsPositive() {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
			   return
		  }
		  gate, err := svc.AdjudicateTransferIn(c.Request.Context(), AdjudicateRequest{
			  LeaseID:  req.LeaseID,
			  UserID:   req.UserID,
			  Symbol:   req.Symbol,
			   Currency: req.Currency,
			   Amount: amt,
			   RequestedBy: req.RequestedBy,
			  TransferID: req.TransferID,
		  })
		  if err != nil {
			   status := http.StatusForbidden
			   c.JSON(status, gin.H{"error": err.Error(), "gate": gate})
			   return
		   }
		  c.JSON(http.StatusCreated, gate)
	   }
  }

  func handleGetGate(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  gateID := strings.TrimSpace(c.Param("gateID"))
		  gate, err := svc.GetGate(c.Request.Context(), gateID)
		  if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, gate)
	   }
   }

// ─────────────────────────────────────────────
// 漂移处理器
// ─────────────────────────────────────────────

   func handleListOpenDrifts(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   userID := strings.TrimSpace(c.Param("userID"))
		   symbol := strings.ToUpper(strings.TrimSpace(c.Param("symbol")))
		  cases, err := svc.ListOpenDriftCases(c.Request.Context(), userID, symbol)
		  if err != nil {
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"items": cases, "count": len(cases)})
	  }
  }

   func handleGetDriftCase(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  caseID := strings.TrimSpace(c.Param("caseID"))
		  dc,
			   err := svc.GetDriftCase(c.Request.Context(), caseID)
		  if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, dc)
	   }
   }

type submitProofReq struct {
	SubmittedBy string `json:"submitted_by" binding:"required"`
	Summary     string `json:"summary"      binding:"required"`
	Evidence string `json:"evidence"`
}

  func handleSubmitProof(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  caseID := strings.TrimSpace(c.Param("caseID"))
		  var req submitProofReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		  proof,
			   err := svc.SubmitDriftProof(c.Request.Context(), caseID, req.SubmittedBy, req.Summary, req.Evidence)
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(http.StatusCreated, proof)
	  }
   }

type confirmCloseReq struct {
	ConfirmedBy string `json:"confirmed_by" binding:"required"`
}

   func handleConfirmClose(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  caseID := strings.TrimSpace(c.Param("caseID"))
		   var req confirmCloseReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  dc, err := svc.ConfirmDriftClose(c.Request.Context(), caseID, req.ConfirmedBy)
		   if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, dc)
	   }
   }

   func handleExportProof(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  proofID := strings.TrimSpace(c.Param("proofID"))
		   proof, err := svc.ExportDriftProof(c.Request.Context(), proofID)
		  if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, proof)
	  }
  }

// ─────────────────────────────────────────────
// ops 处理器
// ─────────────────────────────────────────────

   func handleOpsDebitLocks(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   locks := svc.ActiveDebitLocks()
		  c.JSON(http.StatusOK, gin.H{"active_debit_locks": locks, "count": len(locks)})
	   }
  }

// ─────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────

   func isOverLimitErr(err error) bool {
	   if err == nil {
		  return false
	  }
	   return strings.Contains(err.Error(), "over limit") || strings.Contains(err.Error(), "drift")
  }
