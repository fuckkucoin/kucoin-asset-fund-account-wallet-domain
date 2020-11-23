package swapthaw

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 将 swap-thaw 运营 API 挂载到 Gin RouterGroup。
// routerGroup 应已被 ops 鉴权中间件保护。
  func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	  h := &apiHandler{svc: svc}
	  rg.GET("/generation/:order_id", h.queryGeneration)
	  rg.GET("/lease/:lease_id", h.getLease)
	  rg.GET("/cases", h.listOpenCases)
	  rg.GET("/cases/:case_id", h.getCase)
	   rg.POST("/cases/:case_id/prove", h.proveCase)
	  rg.POST("/cases/:case_id/close", h.closeCase)
	   rg.GET("/proof/export/:case_id", h.exportProof)
  }

type apiHandler struct {
	svc *Service
}

// ──────────────────────────────────────────────
// 冻额世代查询
// ──────────────────────────────────────────────
// queryGeneration 查询订单冻额世代与阻断状态。

  func (h *apiHandler) queryGeneration(c *gin.Context) {
	   orderID := c.Param("order_id")
	  if orderID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "order_id required"})
		   return
	  }
	  resp, err := h.svc.QueryGeneration(orderID)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, resp)
  }

// ──────────────────────────────────────────────
// FreezeLease 查询
// ──────────────────────────────────────────────
// getLease 按 freeze_lease_id 查询 FreezeLease 详情（不含 Token）。
   func (h *apiHandler) getLease(c *gin.Context) {
	  leaseID := c.Param("lease_id")
	  if leaseID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "lease_id required"})
		  return
	   }
	   lease, err := h.svc.GetLease(leaseID)
	   if err != nil {
		  if err == ErrLeaseNotFound {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	   // Token 不得返回给 API 调用方。
	   lease.Token = ""
	   c.JSON(http.StatusOK, lease)
  }

// ──────────────────────────────────────────────
// ThawCase CRUD
// ──────────────────────────────────────────────

type listCasesResp struct {
	Cases []*ThawCase `json:"cases"`
	Total int `json:"total"`
}

// listOpenCases 列出未关闭 ThawCase。
   func (h *apiHandler) listOpenCases(c *gin.Context) {
	   limitStr := c.DefaultQuery("limit", "50")
	  limit, err := strconv.Atoi(limitStr)
	   if err != nil || limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  cases, err := h.svc.ListOpenThawCases(limit)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, listCasesResp{Cases: cases, Total: len(cases)})
   }

// getCase 按 thaw_proof_id 查询 ThawCase 详情。
  func (h *apiHandler) getCase(c *gin.Context) {
	  caseID := c.Param("case_id")
	   if caseID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "case_id required"})
		  return
	  }
	   tc,
		   err := h.svc.GetThawCase(caseID)
	   if err != nil {
		   if err == ErrThawCaseNotFound {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, tc)
  }

// proveReq 证明固化请求体。

type proveReq struct {
	ProofPayload string `json:"proof_payload" binding:"required"`
	Approver string `json:"approver"`
}

// proveCase 固化 ThawCase 证明（open→proven）。
   func (h *apiHandler) proveCase(c *gin.Context) {
	  caseID := c.Param("case_id")
	   if caseID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "case_id required"})
		  return
	  }
	  var req proveReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  tc, err := h.svc.ProveThawCase(ProveThawCaseReq{
		  CaseID:       caseID,
		  ProofPayload: req.ProofPayload,
		  Approver: req.Approver,
	  })
	   if err != nil {
		  switch err {
		  case ErrThawCaseNotFound:
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   case ErrThawCaseProofMissing:
			   c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		  default:
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   }
		  return
	  }
	   c.JSON(http.StatusOK, tc)
  }

// closeReq 收口请求体。
type closeReq struct {
	Approver string `json:"approver"`
}

// closeCase 收口 ThawCase（proven→closed）。

  func (h *apiHandler) closeCase(c *gin.Context) {
	   caseID := c.Param("case_id")
	   if caseID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "case_id required"})
		  return
	  }
	  var req closeReq
	   _ = c.ShouldBindJSON(&req) // 可选字段，绑定失败不阻止
	  tc, err := h.svc.CloseThawCase(CloseThawCaseReq{
		  CaseID: caseID,
		  Approver: req.Approver,
	   })
	  if err != nil {
		  switch err {
		   case ErrThawCaseNotFound:
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  case ErrThawCaseProofMissing:
			  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "proof required before close: " + err.Error()})
		  default:
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  }
		  return
	   }
	   c.JSON(http.StatusOK, tc)
   }

// ──────────────────────────────────────────────
// 证明导出（完整字段，仅限运营）
// ──────────────────────────────────────────────

type proofExportResp struct {
	CaseID  string `json:"thaw_proof_id"`
	OrderID string `json:"order_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	DetectedAmount    string `json:"detected_amount"`
	FrozenSnapshot    string `json:"frozen_snapshot"`
	AvailableSnapshot string `json:"available_snapshot"`
	ProofPayload string `json:"proof_payload"`
	Approver string `json:"approver"`
}

// exportProof 导出完整证明信息（thaw_proof_id），仅限运营接口。

   func (h *apiHandler) exportProof(c *gin.Context) {
	   caseID := c.Param("case_id")
	  if caseID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "case_id required"})
		  return
	   }
	   tc, err := h.svc.GetThawCase(caseID)
	  if err != nil {
		   if err == ErrThawCaseNotFound {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	  resp := proofExportResp{
		  CaseID: tc.ID,
		  OrderID: tc.OrderID,
		  UserID: tc.UserID,
		   Asset: tc.Asset,
		  Kind:              string(tc.Kind),
		  Status:            string(tc.Status),
		  DetectedAmount:    tc.DetectedAmount,
		   FrozenSnapshot:    tc.FrozenSnapshot,
		  AvailableSnapshot: tc.AvailableSnapshot,
		   ProofPayload: tc.ProofPayload,
		   Approver:     tc.Approver,
	  }
	  c.JSON(http.StatusOK, resp)
   }
