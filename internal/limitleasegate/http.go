package limitleasegate

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

// abortForbidden 权限拒绝。

   func abortForbidden(c *gin.Context, err error) {
	   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
   }

// abortBadRequest 请求参数错误。

  func abortBadRequest(c *gin.Context, err error) {
	   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
  }

// Handler HTTP 处理器。
type Handler struct {
	engine       *Engine
	policyWorker *PolicyVersionRevokeWorker
}

// NewHandler 初始化 Handler。

  func NewHandler(engine *Engine, policyWorker *PolicyVersionRevokeWorker) *Handler {
	   return &Handler{engine: engine, policyWorker: policyWorker}
   }

// Register 将所有路由注册到 gin RouterGroup。
   func (h *Handler) Register(rg *gin.RouterGroup) {
	  rg.POST("/limitleasegate/lease/issue", h.issueLease)
	  rg.POST("/limitleasegate/lease/bind", h.bindLease)
	  rg.POST("/limitleasegate/lease/commit-recheck", h.commitRecheck)
	   rg.POST("/limitleasegate/lease/revoke", h.revokeLease)
	  rg.GET("/limitleasegate/lease/:lease_id", h.getLease)
	  rg.GET("/limitleasegate/leases", h.listLeases)
	   rg.POST("/limitleasegate/proof/confirm", h.confirmProof)
	  rg.POST("/limitleasegate/proof/dismiss", h.dismissProof)
	  rg.GET("/limitleasegate/proof/:proof_id", h.getProof)
	   rg.GET("/limitleasegate/proof/:proof_id/export", h.exportProof)
	  rg.GET("/limitleasegate/proofs", h.listProofs)
	   rg.POST("/limitleasegate/policy/notify-version", h.notifyPolicyVersion)
   }

// ─── 租约接口 ─────────────────────────────────────────────────────────────

type issueLeaseReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Kind   string `json:"kind"`
	Amount string `json:"amount"`
	Dest        string `json:"dest,omitempty"`
	Allowlisted bool   `json:"allowlisted,omitempty"`
	IdemKey     string `json:"idem_key,omitempty"`
}

// issueLease POST /limitleasegate/lease/issue
  func (h *Handler) issueLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  var req issueLeaseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	   result, issErr := h.engine.IssueCheckLease(c.Request.Context(), IssueRequest{
		   UserID: req.UserID,
		   Asset:  req.Asset,
		  Kind:   req.Kind,
		  Amount: req.Amount,
		  Dest: req.Dest,
		  Allowlisted: req.Allowlisted,
		  IdemKey:     req.IdemKey,
	  })
	  if issErr != nil && result == nil {
		  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": issErr.Error()})
		  return
	  }
	   if result != nil && !result.Allowed {
		   c.JSON(http.StatusForbidden, gin.H{
			   "allowed": false,
			  "reject_reason": result.RejectReason,
		   })
		   return
	   }
	   c.JSON(http.StatusCreated, gin.H{
		  "allowed": true,
		   "limit_check_lease_id": result.Lease.ID,
		  "policy_ver":         result.Lease.PolicyVer,
		  "today_sum_at_issue": result.Lease.TodaySumAtIssue,
		  "velocity_at_issue":  result.Lease.VelocityAtIssue,
		  "expires_at": result.Lease.ExpiresAt,
		   "state": result.Lease.State,
	   })
  }

type bindLeaseReq struct {
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Asset string `json:"asset"`
	Kind      string `json:"kind"`
	CommitRef string `json:"commit_ref"`
}

// bindLease POST /limitleasegate/lease/bind

   func (h *Handler) bindLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	   var req bindLeaseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	  lease, bindErr := h.engine.BindLease(c.Request.Context(), BindRequest{
		   LeaseID: req.LeaseID,
		   UserID:  req.UserID,
		   Asset:     req.Asset,
		   Kind:      req.Kind,
		  CommitRef: req.CommitRef,
	  })
	  if bindErr != nil {
		   status := http.StatusUnprocessableEntity
		  switch bindErr {
		  case ErrNoLease:
			   status = http.StatusBadRequest
		  case ErrLeaseExpired, ErrLeaseRevoked, ErrLeaseConsumed:
			  status = http.StatusGone
		   case ErrProofBlocked:
			  status = http.StatusConflict
		   case ErrScopeMismatch:
			  status = http.StatusForbidden
		  case ErrNotFound:
			  status = http.StatusNotFound
		  }
		  c.JSON(status, gin.H{"error": bindErr.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		  "lease_id":   lease.ID,
		  "state":      lease.State,
		   "commit_ref": lease.CommitRef,
		   "bound_at": lease.BoundAt,
	  })
   }

type commitRecheckReq struct {
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Kind    string `json:"kind"`
	CommitRef string `json:"commit_ref,omitempty"`
	Allowlisted bool `json:"allowlisted,omitempty"`
}

// commitRecheck POST /limitleasegate/lease/commit-recheck
   func (h *Handler) commitRecheck(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   abortForbidden(c, err)
		  return
	   }
	  var req commitRecheckReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		  return
	  }
	   result, recheckErr := h.engine.CommitRecheck(c.Request.Context(), CommitRecheckRequest{
		   LeaseID: req.LeaseID,
		  UserID:  req.UserID,
		   Asset:   req.Asset,
		  Kind:        req.Kind,
		   CommitRef:   req.CommitRef,
		  Allowlisted: req.Allowlisted,
	  })
	  if recheckErr != nil {
		  if result != nil && !result.Allowed {
			   resp := gin.H{
				  "allowed":       false,
				   "reject_reason": result.RejectReason,
			   }
			  if result.DriftProof != nil {
				  resp["drift_proof_id"] = result.DriftProof.ID
				   resp["drift_type"] = result.DriftProof.DriftType
			   }
			   c.JSON(http.StatusForbidden, resp)
			  return
		  }
		   status := http.StatusUnprocessableEntity
		   switch recheckErr {
		   case ErrNoLease:
			  status = http.StatusBadRequest
		   case ErrLeaseExpired,
			  ErrLeaseRevoked, ErrLeaseConsumed:
			  status = http.StatusGone
		  case ErrNotFound:
			   status = http.StatusNotFound
		  }
		  c.JSON(status, gin.H{"error": recheckErr.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{
		  "allowed":  true,
		   "lease_id": result.Lease.ID,
		  "state":    result.Lease.State,
		  "consumed_at": result.Lease.ConsumedAt,
	  })
   }

type revokeLeaseReq struct {
	LeaseID string `json:"lease_id"`
	Reason  string `json:"reason,omitempty"`
}

// revokeLease POST /limitleasegate/lease/revoke
   func (h *Handler) revokeLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	  var req revokeLeaseReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	   }
	  if req.LeaseID == "" {
		   abortBadRequest(c, ErrNoLease)
		   return
	   }
	   lease, getErr := h.engine.store.GetLeaseByID(c.Request.Context(), req.LeaseID)
	   if getErr != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": getErr.Error()})
		  return
	  }
	   if lease.State.Closed() {
		  c.JSON(http.StatusGone, gin.H{"error": "lease already in terminal state", "state": lease.State})
		  return
	   }
	   now := h.engine.now()
	   t := now
	  lease.State = LeaseStateRevoked
	   lease.RevokedAt = &t
	   if req.Reason != "" {
		  lease.RevokeReason = req.Reason
	  } else {
		  lease.RevokeReason = string(RevokeReasonManual)
	  }
	  lease.UpdatedAt = now
	   if updateErr := h.engine.store.UpdateLease(c.Request.Context(), lease); updateErr != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": updateErr.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "lease_id":      lease.ID,
		  "state":         lease.State,
		   "revoke_reason": lease.RevokeReason,
		   "revoked_at":    lease.RevokedAt,
	   })
   }

// getLease GET /limitleasegate/lease/:lease_id

   func (h *Handler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   leaseID := c.Param("lease_id")
	   lease, getErr := h.engine.store.GetLeaseByID(c.Request.Context(), leaseID)
	   if getErr != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": getErr.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, lease)
   }

// listLeases GET /limitleasegate/leases?user_id=&limit=

  func (h *Handler) listLeases(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	   userID := c.Query("user_id")
	  limit := 20
	  if ls := c.Query("limit"); ls != "" {
		  if n, parseErr := strconv.Atoi(ls); parseErr == nil && n > 0 && n <= 200 {
			  limit = n
		  }
	  }
	  leases, listErr := h.engine.store.ListActiveLeasesByUser(c.Request.Context(), userID, limit)
	  if listErr != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": listErr.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"leases": leases})
   }

// ─── 证明接口 ─────────────────────────────────────────────────────────────

type confirmProofReq struct {
	ProofID string `json:"proof_id"`
	ClosedBy string `json:"closed_by"`
	Notes    string `json:"notes,omitempty"`
}

// confirmProof POST /limitleasegate/proof/confirm

   func (h *Handler) confirmProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	  var req confirmProofReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		  return
	  }
	   if confErr := h.engine.ConfirmProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes); confErr != nil {
		   c.JSON(http.StatusUnprocessableEntity, gin.H{"error": confErr.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proof_id": req.ProofID, "state": "closed"})
   }

type dismissProofReq struct {
	ProofID string `json:"proof_id"`
	ClosedBy string `json:"closed_by"`
	Notes string `json:"notes,omitempty"`
}

// dismissProof POST /limitleasegate/proof/dismiss
  func (h *Handler) dismissProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	   var req dismissProofReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	   if disErr := h.engine.DismissProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes); disErr != nil {
		  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": disErr.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proof_id": req.ProofID, "state": "closed"})
   }

// getProof GET /limitleasegate/proof/:proof_id
   func (h *Handler) getProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	   proofID := c.Param("proof_id")
	   proof, getErr := h.engine.store.GetProofByID(c.Request.Context(), proofID)
	   if getErr != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": getErr.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, proof)
   }

// exportProof GET /limitleasegate/proof/:proof_id/export
   func (h *Handler) exportProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	   proofID := c.Param("proof_id")
	   export,
		   expErr := h.engine.ExportProof(c.Request.Context(), proofID)
	  if expErr != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": expErr.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, export)
  }

// listProofs GET /limitleasegate/proofs?state=&limit=
  func (h *Handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	   state := ProofState(c.Query("state"))
	   if state == "" {
		   state = ProofStateOpen
	  }
	   limit := 20
	   if ls := c.Query("limit"); ls != "" {
		   if n, parseErr := strconv.Atoi(ls); parseErr == nil && n > 0 && n <= 200 {
			   limit = n
		  }
	   }
	   proofs,
		  listErr := h.engine.store.ListProofsByState(c.Request.Context(), state, limit)
	  if listErr != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": listErr.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": proofs})
   }

// ─── 内部通知接口 ─────────────────────────────────────────────────────────

type notifyPolicyVersionReq struct {
	NewPolicyVer int `json:"new_policy_ver"`
}

// notifyPolicyVersion POST /limitleasegate/policy/notify-version
   func (h *Handler) notifyPolicyVersion(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	   var req notifyPolicyVersionReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	   }
	  if req.NewPolicyVer <= 0 {
		   abortBadRequest(c,
			   ErrPolicyDrift)
		   return
	   }
	   if h.policyWorker != nil {
		  h.policyWorker.NotifyPolicyVersion(c.Request.Context(), req.NewPolicyVer)
	   }
	  c.JSON(http.StatusAccepted, gin.H{"new_policy_ver": req.NewPolicyVer, "status": "notify_dispatched"})
  }
