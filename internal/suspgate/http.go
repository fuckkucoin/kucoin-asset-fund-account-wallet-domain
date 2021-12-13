package suspgate

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 把 suspgate API 挂载到 gin RouterGroup。

  func RegisterRoutes(rg *gin.RouterGroup, eng *Engine) {
	   h := &handler{eng: eng}
	   // 写路径（须 ops.admin 权限）。
	   rg.POST("/suspgate/suspend", h.suspend)
	   rg.POST("/suspgate/clear", h.clear)
	  rg.POST("/suspgate/consume-lease", h.consumeLease)
	   rg.POST("/suspgate/withdraw-assert", h.withdrawAssert)
	   // 只读路径。
	  rg.GET("/suspgate/gen/:user_id/:asset", h.getCurrentGen)
	  rg.GET("/suspgate/counter/:user_id/:asset", h.getCounter)
	  rg.GET("/suspgate/lease/:lease_id", h.getLease)
	  rg.GET("/suspgate/proof/:proof_id", h.getProof)
	  rg.GET("/suspgate/proofs/case/:case_id", h.listProofsByCase)
	  rg.GET("/suspgate/proofs/outcome/:outcome", h.listProofsByOutcome)
   }

type handler struct {
	eng *Engine
}

// ──────────────────────────────────────────────────────────────────────────────
// 写路径
// ──────────────────────────────────────────────────────────────────────────────

  func (h *handler) suspend(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var req SuspendWithGenReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		  return
	   }
	  result,
		   err := h.eng.SuspendWithGen(c.Request.Context(), req)
	  if err != nil {
		   statusFor(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{
		   "susp_gen": result.Gen.GenValue,
		  "gen_id": result.Gen.GenID,
		  "state":  result.Gen.State,
		   "active_count": result.Counter.ActiveCount,
	  })
  }

   func (h *handler) clear(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var req ClearWithGenReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		   return
	  }
	   result, err := h.eng.ClearWithGen(c.Request.Context(), req)
	  if err != nil {
		  if errors.Is(err, ErrGenAlreadyCleared) && result != nil && result.DriftProof != nil {
			  c.JSON(http.StatusConflict, gin.H{
				   "error": "gen already cleared",
				   "drift_proof_id": result.DriftProof.ProofID,
				  "block_outcome": result.DriftProof.BlockOutcome,
			   })
			  return
		   }
		  statusFor(c, err)
		  return
	   }
	   resp := gin.H{
		  "gen_id": result.Gen.GenID,
		   "state": result.Gen.State,
		   "susp_gen": result.Gen.GenValue,
	   }
	   if result.Lease != nil {
		   resp["clearance_lease_id"] = result.Lease.LeaseID
		   resp["lease_expire_at"] = result.Lease.ExpireAt
	   }
	  c.JSON(http.StatusOK, resp)
   }

  func (h *handler) consumeLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body struct {
		  LeaseID    string `json:"lease_id"`
		   ConsumedBy string `json:"consumed_by"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		  return
	   }
	  lease, err := h.eng.ConsumeLease(c.Request.Context(), body.LeaseID, body.ConsumedBy)
	  if err != nil {
		   statusFor(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{
		  "lease_id": lease.LeaseID,
		  "state": lease.State,
	   })
  }

  func (h *handler) withdrawAssert(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var req WithdrawAssertReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		  return
	  }
	   result, err := h.eng.WithdrawAssert(c.Request.Context(), req)
	   if err != nil {
		  code := http.StatusConflict
		  if errors.Is(err, ErrLeaseRequired) {
			  code = http.StatusForbidden
		  }
		  c.JSON(code, gin.H{
			  "allowed":      false,
			  "block_reason": result.BlockReason,
			   "active_count": result.ActiveCount,
			  "error":        err.Error(),
		  })
		   return
	   }
	   resp := gin.H{
		  "allowed": result.Allowed,
		   "active_count": result.ActiveCount,
	   }
	  if result.ConsumedLeaseID != "" {
		  resp["consumed_lease_id"] = result.ConsumedLeaseID
	  }
	   c.JSON(http.StatusOK, resp)
   }

// ──────────────────────────────────────────────────────────────────────────────
// 只读路径
// ──────────────────────────────────────────────────────────────────────────────

   func (h *handler) getCurrentGen(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  userID := strings.TrimSpace(c.Param("user_id"))
	   asset := strings.TrimSpace(c.Param("asset"))
	  gen, err := h.eng.GetCurrentGen(c.Request.Context(), userID, asset)
	   if err != nil {
		  statusFor(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gen)
   }

   func (h *handler) getCounter(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   userID := strings.TrimSpace(c.Param("user_id"))
	  asset := strings.TrimSpace(c.Param("asset"))
	   counter, err := h.eng.GetCounter(c.Request.Context(), userID, asset)
	   if err != nil {
		  statusFor(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, counter)
  }

   func (h *handler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   leaseID := strings.TrimSpace(c.Param("lease_id"))
	  lease, err := h.eng.GetLease(c.Request.Context(), leaseID)
	  if err != nil {
		   statusFor(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, lease)
   }

   func (h *handler) getProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   proofID := strings.TrimSpace(c.Param("proof_id"))
	   proof, err := h.eng.GetProof(c.Request.Context(), proofID)
	  if err != nil {
		  statusFor(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, proof)
   }

  func (h *handler) listProofsByCase(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  caseID := strings.TrimSpace(c.Param("case_id"))
	  proofs, err := h.eng.ListProofsByCase(c.Request.Context(), caseID)
	   if err != nil {
		  statusFor(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs})
  }

  func (h *handler) listProofsByOutcome(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   outcome := ProofOutcome(strings.TrimSpace(c.Param("outcome")))
	  limit := 50
	  if lStr := c.Query("limit"); lStr != "" {
		   if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
			  limit = n
		  }
	   }
	   proofs, err := h.eng.ListProofsByOutcome(c.Request.Context(), outcome, limit)
	   if err != nil {
		  statusFor(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs})
  }

// ──────────────────────────────────────────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────────────────────────────────────────

   func statusFor(c *gin.Context, err error) {
	   switch {
	   case errors.Is(err, ErrAsset) || errors.Is(err, ErrActor) || errors.Is(err, ErrInvalidInput):
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrFourEyes):
		   c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	  case errors.Is(err, ErrGenNotFound) || errors.Is(err, ErrLeaseNotFound) || errors.Is(err, ErrCaseNotFound):
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrGenConflict):
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrGenAlreadyCleared):
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrWithdrawBlocked):
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	  case errors.Is(err,
		  ErrLeaseRequired):
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	  case errors.Is(err, ErrLeaseExpired) || errors.Is(err, ErrLeaseConsumed) || errors.Is(err, ErrLeaseInvalid):
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	   default:
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	   }
   }
