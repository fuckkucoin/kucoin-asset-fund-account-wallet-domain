package wdchalgate

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
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
	engine *Engine
}

// NewHandler 初始化 Handler。
   func NewHandler(engine *Engine) *Handler {
	  return &Handler{engine: engine}
  }

// Register 将所有路由注册到 gin RouterGroup。
  func (h *Handler) Register(rg *gin.RouterGroup) {
	  rg.POST("/wdchalgate/lease/issue", h.issueLease)
	   rg.POST("/wdchalgate/lease/present", h.presentLease)
	  rg.POST("/wdchalgate/lease/consume", h.consumeLease)
	  rg.POST("/wdchalgate/lease/revoke", h.revokeLease)
	  rg.GET("/wdchalgate/lease/:lease_id", h.getLease)
	   rg.GET("/wdchalgate/leases", h.listLeases)
	   rg.POST("/wdchalgate/proof/confirm", h.confirmProof)
	   rg.POST("/wdchalgate/proof/dismiss", h.dismissProof)
	   rg.GET("/wdchalgate/proof/:proof_id", h.getProof)
	  rg.GET("/wdchalgate/proof/:proof_id/export", h.exportProof)
	   rg.GET("/wdchalgate/proofs", h.listProofs)
	  rg.POST("/wdchalgate/factor/notify-change", h.notifyFactorChange)
	  rg.POST("/wdchalgate/factor/notify-unlock", h.notifyFailLockUnlock)
	   rg.GET("/wdchalgate/combo-gens", h.listComboGens)
  }

// ─── 租约接口 ─────────────────────────────────────────────────────────────

type issueLeaseReq struct {
	UserID    string `json:"user_id"`
	Factor    string `json:"factor"`
	ExtraKey  string `json:"extra_key"`
	DestChain string `json:"dest_chain"`
	DestAddr string `json:"dest_addr"`
	IdemKey  string `json:"idem_key"`
}

// issueLease POST /wdchalgate/lease/issue
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
	  lease, err := h.engine.IssueLease(c.Request.Context(), IssueRequest{
		  UserID: req.UserID,
		  Factor: req.Factor,
		   ExtraKey: req.ExtraKey,
		  DestChain: req.DestChain,
		   DestAddr:  req.DestAddr,
		  IdemKey:   req.IdemKey,
	   })
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"lease": lease})
   }

type presentLeaseReq struct {
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Factor  string `json:"factor"`
	ExtraKey  string `json:"extra_key"`
	DestChain string `json:"dest_chain"`
	DestAddr string `json:"dest_addr"`
}

// presentLease POST /wdchalgate/lease/present
   func (h *Handler) presentLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	   var req presentLeaseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		   return
	  }
	  result, err := h.engine.PresentLease(c.Request.Context(), PresentRequest{
		  LeaseID: req.LeaseID,
		   UserID: req.UserID,
		   Factor:    req.Factor,
		  ExtraKey:  req.ExtraKey,
		  DestChain: req.DestChain,
		   DestAddr:  req.DestAddr,
	  })
	   if err != nil {
		   // ErrLeaseStale 时仍返回 result（含 proof_id），HTTP 409 供上层感知漂移。
		  if result != nil && result.Drifted {
			   c.JSON(http.StatusConflict, gin.H{
				  "error":   err.Error(),
				  "drifted": true,
				   "proof_id": result.ProofID,
				   "lease":    result.Lease,
			  })
			  return
		   }
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"lease": result.Lease, "drifted": false})
   }

type consumeLeaseReq struct {
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
}

// consumeLease POST /wdchalgate/lease/consume
  func (h *Handler) consumeLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	  var req consumeLeaseReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	   lease, err := h.engine.ConsumeLease(c.Request.Context(), req.LeaseID, req.UserID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"lease": lease})
   }

type revokeLeaseReq struct {
	LeaseID string `json:"lease_id"`
	UserID string `json:"user_id"`
	Reason string `json:"reason"`
}

// revokeLease POST /wdchalgate/lease/revoke

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
	   lease, err := h.engine.RevokeLease(c.Request.Context(), req.LeaseID, req.UserID, req.Reason)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"lease": lease})
   }

// getLease GET /wdchalgate/lease/:lease_id

   func (h *Handler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		   return
	   }
	   lease, err := h.engine.GetLease(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"lease": lease})
   }

// listLeases GET /wdchalgate/leases?user_id=&limit=

   func (h *Handler) listLeases(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	  userID := strings.TrimSpace(c.Query("user_id"))
	  limit := parseLimit(c.Query("limit"), 50)
	   leases, err := h.engine.ListOpenLeases(c.Request.Context(), userID, limit)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"leases": leases})
   }

// ─── 证明接口 ─────────────────────────────────────────────────────────────

type confirmProofReq struct {
	ProofID string `json:"proof_id"`
	ClosedBy string `json:"closed_by"`
	Notes    string `json:"notes"`
}

// confirmProof POST /wdchalgate/proof/confirm
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
	   proof, err := h.engine.ConfirmProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proof": proof})
  }

type dismissProofReq struct {
	ProofID string `json:"proof_id"`
	ClosedBy string `json:"closed_by"`
	Notes string `json:"notes"`
}

// dismissProof POST /wdchalgate/proof/dismiss
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
	  proof, err := h.engine.DismissProof(c.Request.Context(), req.ProofID, req.ClosedBy, req.Notes)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"proof": proof})
  }

// getProof GET /wdchalgate/proof/:proof_id
  func (h *Handler) getProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	   proof, err := h.engine.GetProof(c.Request.Context(), c.Param("proof_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proof": proof})
  }

// exportProof GET /wdchalgate/proof/:proof_id/export
   func (h *Handler) exportProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  summary, err := h.engine.ExportProof(c.Request.Context(), c.Param("proof_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"summary": summary})
  }

// listProofs GET /wdchalgate/proofs?state=&limit=

  func (h *Handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   abortForbidden(c, err)
		  return
	  }
	  state := ProofState(strings.TrimSpace(c.Query("state")))
	   if state == "" {
		  state = ProofStateOpen
	  }
	   limit := parseLimit(c.Query("limit"), 50)
	   proofs, err := h.engine.ListProofsByState(c.Request.Context(), state, limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs})
   }

// ─── 因子通知接口（内部） ─────────────────────────────────────────────────

type notifyFactorChangeReq struct {
	UserID string `json:"user_id"`
	Factor string `json:"factor"`
	ExtraKey string `json:"extra_key"`
	ChangeReason string `json:"change_reason"`
	Actor string `json:"actor"`
	RefID string `json:"ref_id"`
}

// notifyFactorChange POST /wdchalgate/factor/notify-change
  func (h *Handler) notifyFactorChange(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	  var req notifyFactorChangeReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  abortBadRequest(c, err)
		   return
	  }
	   gen, err := h.engine.NotifyFactorChange(c.Request.Context(), FactorChangeEvent{
		   UserID: req.UserID,
		   Factor: req.Factor,
		   ExtraKey: req.ExtraKey,
		   ChangeReason: ChangeReason(req.ChangeReason),
		  Actor:        req.Actor,
		   RefID: req.RefID,
	   })
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"combo_gen": gen})
   }

type notifyFailLockUnlockReq struct {
	UserID string `json:"user_id"`
	Actor  string `json:"actor"`
}

// notifyFailLockUnlock POST /wdchalgate/factor/notify-unlock

   func (h *Handler) notifyFailLockUnlock(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  abortForbidden(c, err)
		   return
	  }
	   var req notifyFailLockUnlockReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   abortBadRequest(c, err)
		  return
	   }
	  gen, err := h.engine.NotifyFailLockUnlock(c.Request.Context(), req.UserID, req.Actor)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"combo_gen": gen})
   }

// listComboGens GET /wdchalgate/combo-gens?user_id=&limit=
   func (h *Handler) listComboGens(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  abortForbidden(c, err)
		   return
	   }
	  userID := strings.TrimSpace(c.Query("user_id"))
	   limit := parseLimit(c.Query("limit"), 50)
	   gens, err := h.engine.ListComboGens(c.Request.Context(), userID, limit)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"combo_gens": gens})
  }

// ─── 辅助 ─────────────────────────────────────────────────────────────────

   func parseLimit(s string, def int) int {
	  if s == "" {
		   return def
	  }
	  n, err := strconv.Atoi(s)
	   if err != nil || n <= 0 {
		  return def
	   }
	   if n > 500 {
		  return 500
	  }
	  return n
   }
