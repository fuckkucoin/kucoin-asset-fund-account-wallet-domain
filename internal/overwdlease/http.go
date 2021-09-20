// overwdlease HTTP / Ops API。
// 端点：
// POST /overwdlease/bind/issue — 签发预检绑定租约（PreCheck 通过后调用）
// POST /overwdlease/bind/commit — Commit 前复验绑定（LeaseGen/Available 核验）
// POST /overwdlease/bind/revoke — 手动吊销 open bind
// GET /overwdlease/bind/:bind_id — 查询绑定租约状态
// GET /overwdlease/binds — 列举用户 open 绑定
// POST /overwdlease/proof/confirm — 四眼确认差异证明（关闭挡写）
// POST /overwdlease/proof/dismiss — 驳回差异证明
// GET /overwdlease/proof/:proof_id — 查询差异证明状态
// GET /overwdlease/proof/:proof_id/export — 导出证明审计摘要
// GET /overwdlease/proofs — 列举差异证明（按状态）
// GET /overwdlease/drift-case/:case_id — 查询漂移案例
// GET /overwdlease/drift-cases — 漂移案例列表（按状态）
// POST /overwdlease/drift-case/confirm — 管理员确认漂移案例
// POST /overwdlease/drift-case/dismiss — 驳回漂移案例
// 权限：
// - 读取接口：ActCaseRead / ActOpsAdmin
// - 签发/Commit：ActAccountMove（业务层）
// - 四眼确认/关案：ActOpsAdmin（必须两人不同 ID）

// Author: JaxRoe
// Date: 2021.09.19

package overwdlease

import (
	"net/http"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

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
	  rg.POST("/overwdlease/bind/issue", h.issueBind)
	   rg.POST("/overwdlease/bind/commit", h.commitBind)
	  rg.POST("/overwdlease/bind/revoke", h.revokeBind)
	   rg.GET("/overwdlease/bind/:bind_id", h.getBind)
	   rg.GET("/overwdlease/binds", h.listBinds)
	   rg.POST("/overwdlease/proof/confirm", h.confirmProof)
	  rg.POST("/overwdlease/proof/dismiss", h.dismissProof)
	  rg.GET("/overwdlease/proof/:proof_id", h.getProof)
	  rg.GET("/overwdlease/proof/:proof_id/export", h.exportProof)
	  rg.GET("/overwdlease/proofs", h.listProofs)
	   rg.GET("/overwdlease/drift-case/:case_id", h.getDriftCase)
	   rg.GET("/overwdlease/drift-cases", h.listDriftCases)
	   rg.POST("/overwdlease/drift-case/confirm", h.confirmDriftCase)
	   rg.POST("/overwdlease/drift-case/dismiss", h.dismissDriftCase)
   }

// ─── 绑定接口 ─────────────────────────────────────────────────────────────

type issueBindReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount   string `json:"amount"`
	LeaseGen int64  `json:"lease_gen"`
	Available string `json:"available"`
	IdemKey   string `json:"idem_key"`
}

// issueBind POST /overwdlease/bind/issue

  func (h *Handler) issueBind(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var req issueBindReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   reply, err := h.engine.IssueBind(c.Request.Context(), IssueBindRequest{
		   UserID: req.UserID,
		   Asset:  req.Asset,
		   Amount: req.Amount,
		  Decision: BarrierDecision{
			   Allowed:  true,
			   LeaseGen: req.LeaseGen,
			   Available: req.Available,
		  },
		  IdemKey: req.IdemKey,
	  })
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "bind_id": reply.BindID,
		   "lease_gen":  reply.LeaseGen,
		   "expires_at": reply.ExpiresAt.Format(time.RFC3339),
		   "state": reply.State,
	  })
  }

type commitBindReq struct {
	BindID string `json:"bind_id"`
	UserID string `json:"user_id"`
	Asset     string `json:"asset"`
	CommitRef string `json:"commit_ref"`
	CurrentGen int64 `json:"current_gen"`
	CurrentAvail string `json:"current_avail"`
}

// commitBind POST /overwdlease/bind/commit

  func (h *Handler) commitBind(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var req commitBindReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   reply, err := h.engine.CommitBind(c.Request.Context(), CommitBindRequest{
		   BindID: req.BindID,
		  UserID:    req.UserID,
		  Asset:     req.Asset,
		  CommitRef: req.CommitRef,
		  Current: LeaseObservation{
			   UserID:     req.UserID,
			  Asset:      req.Asset,
			  LeaseGen:   req.CurrentGen,
			   Available:  req.CurrentAvail,
			   ObservedAt: time.Now(),
		   },
	   })
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "bind_id":   reply.BindID,
		  "lease_gen": reply.LeaseGen,
		   "state":     reply.State,
	   })
   }

type revokeBindReq struct {
	BindID string `json:"bind_id"`
	Reason string `json:"reason"`
}

// revokeBind POST /overwdlease/bind/revoke
  func (h *Handler) revokeBind(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var req revokeBindReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	  if err := h.engine.RevokeBind(c.Request.Context(), req.BindID, req.Reason); err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"status": "revoked", "bind_id": req.BindID})
  }

// getBind GET /overwdlease/bind/:bind_id

  func (h *Handler) getBind(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   bindID := strings.TrimSpace(c.Param("bind_id"))
	   b, err := h.engine.GetBind(c.Request.Context(), bindID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK,
		  buildBindResp(b))
   }

// listBinds GET /overwdlease/binds?user_id=&asset=&limit=

  func (h *Handler) listBinds(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  userID := strings.TrimSpace(c.Query("user_id"))
	  asset := strings.TrimSpace(c.Query("asset"))
	   limit := 50
	   binds, err := h.engine.store.ListOpenBindsByUser(c.Request.Context(), userID, asset, limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  out := make([]gin.H, 0, len(binds))
	   for _, b := range binds {
		  out = append(out, buildBindResp(b))
	   }
	   c.JSON(http.StatusOK, gin.H{"binds": out, "count": len(out)})
   }

// ─── 差异证明接口 ─────────────────────────────────────────────────────────

type confirmProofReq struct {
	ProofID     string `json:"proof_id"`
	ConfirmedBy string `json:"confirmed_by"`
	CloseReason string `json:"close_reason"`
}

// confirmProof POST /overwdlease/proof/confirm
// 四眼确认差异证明：关闭挡写，允许同用户同资产再次 Commit。
// 要求：ConfirmedBy 必须与开证明时的触发人（bind.UserID 所属操作人）不同。

   func (h *Handler) confirmProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var req confirmProofReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  if err := h.engine.ConfirmProof(c.Request.Context(), ConfirmProofRequest{
		   ProofID:     req.ProofID,
		   ConfirmedBy: req.ConfirmedBy,
		   CloseReason: req.CloseReason,
	   }); err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"status": "closed", "proof_id": req.ProofID})
  }

type dismissProofReq struct {
	ProofID     string `json:"proof_id"`
	DismissedBy string `json:"dismissed_by"`
	Reason string `json:"reason"`
}

// dismissProof POST /overwdlease/proof/dismiss
   func (h *Handler) dismissProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var req dismissProofReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  if err := h.engine.DismissProof(c.Request.Context(), req.ProofID, req.DismissedBy, req.Reason); err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"status": "dismissed", "proof_id": req.ProofID})
   }

// getProof GET /overwdlease/proof/:proof_id

   func (h *Handler) getProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  proofID := strings.TrimSpace(c.Param("proof_id"))
	  p, err := h.engine.GetProof(c.Request.Context(), proofID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, buildProofResp(p))
  }

// exportProof GET /overwdlease/proof/:proof_id/export

  func (h *Handler) exportProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   proofID := strings.TrimSpace(c.Param("proof_id"))
	   export, err := h.engine.ExportProof(c.Request.Context(), proofID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, export)
  }

// listProofs GET /overwdlease/proofs?state=&limit=

   func (h *Handler) listProofs(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   stateStr := strings.TrimSpace(c.DefaultQuery("state", "open"))
	  state := ProofState(stateStr)
	  limit := 50
	   proofs, err := h.engine.store.ListProofsByState(c.Request.Context(), state, limit)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   out := make([]gin.H, 0, len(proofs))
	   for _, p := range proofs {
		  out = append(out, buildProofResp(p))
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": out, "count": len(out)})
   }

// ─── 漂移案例接口 ─────────────────────────────────────────────────────────
// getDriftCase GET /overwdlease/drift-case/:case_id
  func (h *Handler) getDriftCase(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   caseID := strings.TrimSpace(c.Param("case_id"))
	  dc, err := h.engine.GetDriftCase(c.Request.Context(), caseID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, buildDriftCaseResp(dc))
   }

// listDriftCases GET /overwdlease/drift-cases?state=&limit=
  func (h *Handler) listDriftCases(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   stateStr := strings.TrimSpace(c.DefaultQuery("state", "open"))
	   state := DriftCaseState(stateStr)
	  limit := 50
	   cases, err := h.engine.store.ListDriftCasesByState(c.Request.Context(), state, limit)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  out := make([]gin.H, 0, len(cases))
	   for _, dc := range cases {
		  out = append(out, buildDriftCaseResp(dc))
	  }
	  c.JSON(http.StatusOK, gin.H{"cases": out, "count": len(out)})
  }

type confirmDriftCaseReq struct {
	CaseID      string `json:"case_id"`
	ConfirmedBy string `json:"confirmed_by"`
	CloseReason string `json:"close_reason"`
}

// confirmDriftCase POST /overwdlease/drift-case/confirm
   func (h *Handler) confirmDriftCase(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var req confirmDriftCaseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	   if err := h.engine.ConfirmDriftCase(c.Request.Context(), ConfirmDriftCaseRequest{
		  CaseID: req.CaseID,
		  ConfirmedBy: req.ConfirmedBy,
		   CloseReason: req.CloseReason,
	   }); err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"status": "closed", "case_id": req.CaseID})
  }

type dismissDriftCaseReq struct {
	CaseID string `json:"case_id"`
	DismissedBy string `json:"dismissed_by"`
	Reason string `json:"reason"`
}

// dismissDriftCase POST /overwdlease/drift-case/dismiss

  func (h *Handler) dismissDriftCase(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var req dismissDriftCaseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	  if err := h.engine.DismissDriftCase(c.Request.Context(), req.CaseID, req.DismissedBy, req.Reason); err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"status": "dismissed", "case_id": req.CaseID})
  }

// ─── 辅助函数 ─────────────────────────────────────────────────────────────

   func buildBindResp(b *PrecheckBind) gin.H {
	  resp := gin.H{
		   "bind_id": b.ID,
		   "user_id": b.UserID,
		  "asset":   b.Asset,
		  "amount":  b.Amount,
		   "lease_gen":      b.LeaseGen,
		  "expected_avail": b.ExpectedAvail,
		  "state":          b.State,
		   "bind_hash": b.BindHash,
		  "precheck_at": b.PreCheckAt.Format(time.RFC3339),
		   "expires_at":  b.ExpiresAt.Format(time.RFC3339),
	   }
	  if b.CommittedAt != nil {
		   resp["committed_at"] = b.CommittedAt.Format(time.RFC3339)
		   resp["commit_ref"] = b.CommitRef
	   }
	   if b.RevokeReason != "" {
		   resp["revoke_reason"] = b.RevokeReason
	   }
	   if b.ProofID != "" {
		   resp["proof_id"] = b.ProofID
	  }
	  return resp
  }

  func buildProofResp(p *VarianceProof) gin.H {
	  resp := gin.H{
		  "proof_id": p.ID,
		   "bind_id": p.BindID,
		  "user_id": p.UserID,
		  "asset": p.Asset,
		   "trigger": p.Trigger,
		   "bind_lease_gen": p.BindLeaseGen,
		   "commit_gen": p.CommitGen,
		   "drift_gen":  p.DriftGen,
		   "bind_avail": p.BindAvail,
		  "commit_avail": p.CommitAvail,
		   "state": p.State,
		  "proof_hash": p.ProofHash,
		   "sealed_at":        p.SealedAt.Format(time.RFC3339),
		  "drift_case_id":    p.DriftCaseID,
		   "live_hash_freeze": p.LiveHashFreeze,
	   }
	   if p.ConfirmedAt != nil {
		  resp["confirmed_at"] = p.ConfirmedAt.Format(time.RFC3339)
		   resp["confirmed_by"] = p.ConfirmedBy
		   resp["close_reason"] = p.CloseReason
	   }
	   return resp
  }

  func buildDriftCaseResp(dc *OverwdDriftCase) gin.H {
	   resp := gin.H{
		   "case_id": dc.ID,
		  "user_id": dc.UserID,
		  "asset":   dc.Asset,
		   "trigger": dc.Trigger,
		   "bind_id": dc.BindID,
		   "proof_id": dc.ProofID,
		  "state": dc.State,
		   "drift_gen": dc.DriftGen,
		  "blocked_at": dc.BlockedAt.Format(time.RFC3339),
		  "scan_note":  dc.ScanNote,
	  }
	   if dc.ConfirmedAt != nil {
		  resp["confirmed_at"] = dc.ConfirmedAt.Format(time.RFC3339)
		  resp["confirmed_by"] = dc.ConfirmedBy
		  resp["close_reason"] = dc.CloseReason
	   }
	  return resp
   }
