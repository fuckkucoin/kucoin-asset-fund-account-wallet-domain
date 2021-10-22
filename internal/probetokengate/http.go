package probetokengate

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/writegate"
)

// ─── Handler ──────────────────────────────────────────────────────────────
// Handler HTTP 处理器。
type Handler struct {
	engine *Engine
}

// NewHandler 初始化 Handler。
   func NewHandler(engine *Engine) *Handler {
	   return &Handler{engine: engine}
   }

// Register 将所有路由注册到 gin RouterGroup。
   func (h *Handler) Register(rg *gin.RouterGroup) {
	   rg.POST("/probetokengate/check", h.jointCheck)
	   rg.POST("/probetokengate/lease/bind", h.bindLease)
	   rg.POST("/probetokengate/lease/commit-check", h.commitCheck)
	  rg.POST("/probetokengate/lease/revoke", h.revokeLease)
	   rg.GET("/probetokengate/lease/:lease_id", h.getLease)
	  rg.GET("/probetokengate/leases", h.listLeases)
	   rg.POST("/probetokengate/proof/confirm", h.confirmProof)
	   rg.POST("/probetokengate/proof/dismiss", h.dismissProof)
	  rg.GET("/probetokengate/proof/:proof_id", h.getProof)
	   rg.GET("/probetokengate/proof/:proof_id/export", h.exportProof)
	   rg.GET("/probetokengate/proofs", h.listProofs)
	  rg.POST("/probetokengate/event/probe-flip", h.notifyProbeFlip)
	   rg.POST("/probetokengate/event/poison-revoke", h.notifyPoisonRevoke)
	   rg.GET("/probetokengate/bind/:lease_id", h.getBind)
  }

// ─── 辅助 ─────────────────────────────────────────────────────────────────

   func abortBadRequest(c *gin.Context, err error) {
	   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
   }

   func abortForbidden(c *gin.Context, err error) {
	  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
  }

// ─── 联合 Check 端点 ───────────────────────────────────────────────────────

type jointCheckReq struct {
	ScopeKind string `json:"scope_kind" binding:"required"`
	SubjectID string `json:"subject_id" binding:"required"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	IdemKey string `json:"idem_key"`
}

type jointCheckResp struct {
	Allowed      bool   `json:"allowed"`
	LeaseID      string `json:"lease_id,omitempty"`
	RejectReason string `json:"reject_reason,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

   func (h *Handler) jointCheck(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	  if err := actor.Require(perm.ActWithdrawSubmit); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	  var req jointCheckReq
	   if !mw.BindJSON(c, &req) {
		  return
	   }
	   scope := writegate.CheckScope{
		  Kind:      writegate.ScopeKind(req.ScopeKind),
		  SubjectID: req.SubjectID,
		  Chain:     req.Chain,
		  Asset: req.Asset,
	   }
	  result, err := h.engine.JointCheck(c.Request.Context(), JointCheckRequest{
		  Scope: scope,
		   IdemKey: req.IdemKey,
	  })
	   if err != nil && result == nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   if result == nil || !result.Allowed {
		  reason := ""
		   if result != nil {
			  reason = result.RejectReason
		   }
		  c.JSON(http.StatusForbidden, jointCheckResp{Allowed: false, RejectReason: reason})
		   return
	  }
	   resp := jointCheckResp{
		   Allowed: true,
		   LeaseID: result.Lease.ID,
	  }
	   if !result.Lease.ExpiresAt.IsZero() {
		  resp.ExpiresAt = result.Lease.ExpiresAt.Format("2006-01-02T15:04:05Z")
	  }
	  mw.WriteOK(c, resp)
  }

// ─── BindLease 端点 ───────────────────────────────────────────────────────

type bindLeaseReq struct {
	LeaseID string `json:"lease_id" binding:"required"`
	CommitRef string `json:"commit_ref" binding:"required"`
	ScopeKind string `json:"scope_kind"`
	SubjectID string `json:"subject_id"`
	Chain     string `json:"chain"`
	Asset     string `json:"asset"`
}

type bindLeaseResp struct {
	LeaseID string `json:"lease_id"`
	State     string `json:"state"`
	CommitRef string `json:"commit_ref"`
	BindID string `json:"bind_id"`
}

  func (h *Handler) bindLease(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	   if err := actor.Require(perm.ActWithdrawSubmit); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	   var req bindLeaseReq
	  if !mw.BindJSON(c, &req) {
		   return
	  }
	  scope := writegate.CheckScope{
		  Kind:      writegate.ScopeKind(req.ScopeKind),
		   SubjectID: req.SubjectID,
		  Chain:     req.Chain,
		  Asset:     req.Asset,
	  }
	   result, err := h.engine.BindLease(c.Request.Context(), BindRequest{
		   LeaseID: req.LeaseID,
		  CommitRef: req.CommitRef,
		  Scope:     scope,
	  })
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  mw.WriteOK(c, bindLeaseResp{
		  LeaseID: result.Lease.ID,
		  State:   string(result.Lease.State),
		   CommitRef: result.Lease.CommitRef,
		  BindID:    result.Bind.ID,
	   })
  }

// ─── CommitCheck 端点 ─────────────────────────────────────────────────────

type commitCheckReq struct {
	LeaseID   string `json:"lease_id" binding:"required"`
	CommitRef string `json:"commit_ref"`
	ScopeKind string `json:"scope_kind"`
	SubjectID string `json:"subject_id"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
}

type commitCheckResp struct {
	LeaseID string `json:"lease_id"`
	State   string `json:"state"`
	Drifted bool   `json:"drifted"`
	DriftProofID string `json:"drift_proof_id,omitempty"`
	DriftType    string `json:"drift_type,omitempty"`
}

   func (h *Handler) commitCheck(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	  if err := actor.Require(perm.ActWithdrawSubmit); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	   var req commitCheckReq
	   if !mw.BindJSON(c, &req) {
		  return
	  }
	  scope := writegate.CheckScope{
		  Kind:      writegate.ScopeKind(req.ScopeKind),
		   SubjectID: req.SubjectID,
		  Chain:     req.Chain,
		   Asset: req.Asset,
	  }
	   result, err := h.engine.CommitCheck(c.Request.Context(), CommitCheckRequest{
		   LeaseID:   req.LeaseID,
		   CommitRef: req.CommitRef,
		  Scope:     scope,
	   })
	   if err != nil {
		   if result != nil && result.Drifted {
			   c.JSON(http.StatusConflict, commitCheckResp{
				   LeaseID: req.LeaseID,
				  State:   string(LeaseStateRevoked),
				  Drifted:      true,
				   DriftProofID: result.DriftProofID,
				   DriftType: string(result.DriftType),
			  })
			  return
		   }
		   mw.WriteProblem(c, err)
		   return
	  }
	  state := ""
	  if result.Lease != nil {
		  state = string(result.Lease.State)
	   }
	  mw.WriteOK(c, commitCheckResp{
		   LeaseID: req.LeaseID,
		   State:   state,
		   Drifted: false,
	  })
  }

// ─── RevokeLease 端点 ─────────────────────────────────────────────────────

type revokeLeaseReq struct {
	LeaseID string `json:"lease_id" binding:"required"`
	Reason  string `json:"reason"`
}

  func (h *Handler) revokeLease(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	   if err := actor.Require(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	  var req revokeLeaseReq
	  if !mw.BindJSON(c, &req) {
		  return
	  }
	  lease, err := h.engine.RevokeLease(c.Request.Context(), req.LeaseID, req.Reason)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   mw.WriteOK(c, gin.H{"lease_id": lease.ID, "state": string(lease.State)})
  }

// ─── GetLease 端点 ────────────────────────────────────────────────────────

  func (h *Handler) getLease(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	  if err := actor.Require(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  leaseID := c.Param("lease_id")
	  if leaseID == "" {
		  abortBadRequest(c, ErrNoLease)
		  return
	   }
	   lease, err := h.engine.GetLease(c.Request.Context(), leaseID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  mw.WriteOK(c, lease)
  }

// ─── ListLeases 端点 ──────────────────────────────────────────────────────

   func (h *Handler) listLeases(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	   if err := actor.Require(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	  scopeKey := c.Query("scope_key")
	   limitStr := c.DefaultQuery("limit", "50")
	   limit, _ := strconv.Atoi(limitStr)
	   if limit <= 0 || limit > 500 {
		  limit = 50
	   }
	   leases, err := h.engine.ListActiveLeasesByScope(c.Request.Context(), scopeKey, limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   mw.WriteOK(c, gin.H{"leases": leases, "count": len(leases)})
  }

// ─── Proof 端点 ───────────────────────────────────────────────────────────

type proofActionReq struct {
	ProofID  string `json:"proof_id" binding:"required"`
	ClosedBy string `json:"closed_by"`
	Notes string `json:"notes"`
}

   func (h *Handler) confirmProof(c *gin.Context) {
	   actor := mw.ActorFrom(c)
	   if err := actor.Require(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	  var req proofActionReq
	  if !mw.BindJSON(c, &req) {
		  return
	  }
	   p,
		   err := h.engine.ConfirmProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  mw.WriteOK(c, p)
  }

  func (h *Handler) dismissProof(c *gin.Context) {
	   actor := mw.ActorFrom(c)
	  if err := actor.Require(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	   var req proofActionReq
	   if !mw.BindJSON(c, &req) {
		   return
	  }
	   p,
		  err := h.engine.DismissProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   mw.WriteOK(c, p)
  }

  func (h *Handler) getProof(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	   if err := actor.Require(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		   return
	  }
	  proofID := c.Param("proof_id")
	   p, err := h.engine.GetProof(c.Request.Context(), proofID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  mw.WriteOK(c, p)
   }

   func (h *Handler) exportProof(c *gin.Context) {
	   actor := mw.ActorFrom(c)
	  if err := actor.Require(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	   proofID := c.Param("proof_id")
	  summary, err := h.engine.ExportProof(c.Request.Context(), proofID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  mw.WriteOK(c, summary)
  }

  func (h *Handler) listProofs(c *gin.Context) {
	   actor := mw.ActorFrom(c)
	   if err := actor.Require(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		  return
	   }
	  stateStr := c.DefaultQuery("state", "open")
	  limitStr := c.DefaultQuery("limit", "50")
	  limit, _ := strconv.Atoi(limitStr)
	  if limit <= 0 || limit > 500 {
		  limit = 50
	   }
	  proofs, err := h.engine.ListProofsByState(c.Request.Context(), ProofState(stateStr), limit)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   mw.WriteOK(c, gin.H{"proofs": proofs, "count": len(proofs)})
  }

// ─── 事件通知端点（内部） ──────────────────────────────────────────────────

type probeFlipEventReq struct {
	ScopeKey string `json:"scope_key"`
	PairID              string `json:"pair_id"`
	NewProbeFingerprint string `json:"new_probe_fingerprint"`
	Actor string `json:"actor"`
}

   func (h *Handler) notifyProbeFlip(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	  if err := actor.Require(perm.ActOpsAdmin); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	   var req probeFlipEventReq
	   if !mw.BindJSON(c, &req) {
		  return
	   }
	  n, err := h.engine.RevokeOnProbeFlip(c.Request.Context(), ProbeFlipEvent{
		   ScopeKey:            req.ScopeKey,
		  PairID:              req.PairID,
		   NewProbeFingerprint: req.NewProbeFingerprint,
		  Actor:               req.Actor,
	   })
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   mw.WriteOK(c, gin.H{"revoked": n})
  }

type poisonRevokeEventReq struct {
	ScopeKey           string `json:"scope_key"`
	NewTokenGeneration int64  `json:"new_token_generation"`
	Actor string `json:"actor"`
}

  func (h *Handler) notifyPoisonRevoke(c *gin.Context) {
	   actor := mw.ActorFrom(c)
	  if err := actor.Require(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		  return
	  }
	  var req poisonRevokeEventReq
	  if !mw.BindJSON(c, &req) {
		   return
	  }
	   n, err := h.engine.RevokeOnPoisonRevoke(c.Request.Context(), PoisonRevokeEvent{
		   ScopeKey: req.ScopeKey,
		   NewTokenGeneration: req.NewTokenGeneration,
		   Actor: req.Actor,
	   })
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   mw.WriteOK(c, gin.H{"revoked": n})
  }

// ─── GetBind 端点 ─────────────────────────────────────────────────────────

   func (h *Handler) getBind(c *gin.Context) {
	  actor := mw.ActorFrom(c)
	   if err := actor.Require(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	   leaseID := c.Param("lease_id")
	   bind, err := h.engine.GetBind(c.Request.Context(), leaseID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   mw.WriteOK(c, bind)
   }
