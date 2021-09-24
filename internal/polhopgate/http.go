package polhopgate

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
	   rg.GET("/polhopgate/token/:token_id", h.getToken)
	  rg.GET("/polhopgate/tokens", h.listTokens)
	   rg.POST("/polhopgate/token/issue", h.issueToken)
	  rg.POST("/polhopgate/token/bind", h.bindHop)
	   rg.POST("/polhopgate/lease/acquire", h.acquireLease)
	  rg.POST("/polhopgate/lease/release", h.releaseLease)
	  rg.POST("/polhopgate/dualread/record", h.recordDualRead)
	   rg.GET("/polhopgate/drift-case/:case_id", h.getDriftCase)
	   rg.GET("/polhopgate/drift-cases", h.listDriftCases)
	  rg.POST("/polhopgate/drift-case/confirm", h.confirmDriftCase)
	  rg.POST("/polhopgate/drift-case/dismiss", h.dismissDriftCase)
	  rg.POST("/polhopgate/force-re-evaluate", h.forceReEvaluate)
	   rg.GET("/polhopgate/escalate", h.getEscalate)
  }

// ─── 令牌接口 ─────────────────────────────────────────────────────────────
// getToken GET /polhopgate/token/:token_id

   func (h *Handler) getToken(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   tokenID := strings.TrimSpace(c.Param("token_id"))
	   if tokenID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "token_id required"})
		  return
	   }
	  tok, err := h.engine.GetToken(c.Request.Context(), tokenID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "token_id": tok.ID,
		  "user_id": tok.UserID,
		  "asset":   tok.Asset,
		   "biz_kind": tok.BizKind,
		  "policy_gen": tok.PolicyGen,
		  "policy_id":  tok.PolicyID,
		   "state":      tok.State,
		  "issued_at":  tok.IssuedAt.Format(time.RFC3339),
		   "expires_at": tok.ExpiresAt.Format(time.RFC3339),
		   "hop_ref": tok.HopRef,
	   })
  }

// listTokens GET /polhopgate/tokens?user_id=&biz_kind=&limit=
  func (h *Handler) listTokens(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  userID := strings.TrimSpace(c.Query("user_id"))
	   bizKind := strings.TrimSpace(c.Query("biz_kind"))
	   limit := 50
	   tokens, err := h.engine.store.ListTokensByUser(c.Request.Context(), userID, bizKind, limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   out := make([]gin.H,
		  0, len(tokens))
	   for _, tok := range tokens {
		   out = append(out, gin.H{
			   "token_id": tok.ID,
			   "biz_kind": tok.BizKind,
			  "policy_gen": tok.PolicyGen,
			   "state":      tok.State,
			  "issued_at":  tok.IssuedAt.Format(time.RFC3339),
			  "expires_at": tok.ExpiresAt.Format(time.RFC3339),
		   })
	  }
	   c.JSON(http.StatusOK, gin.H{"tokens": out, "count": len(out)})
  }

type issueTokenReq struct {
	UserID string `json:"user_id"`
	Asset   string `json:"asset"`
	BizKind string `json:"biz_kind"`
	IdemKey string `json:"idem_key"`
}

// issueToken POST /polhopgate/token/issue
  func (h *Handler) issueToken(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var req issueTokenReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   reply, err := h.engine.IssueToken(c.Request.Context(), IssueRequest{
		  UserID: req.UserID,
		  Asset: req.Asset,
		  BizKind: req.BizKind,
		   IdemKey: req.IdemKey,
	  })
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{
		   "token_id": reply.TokenID,
		  "policy_gen": reply.PolicyGen,
		  "policy_id":  reply.PolicyID,
		   "expires_at": reply.ExpiresAt.Format(time.RFC3339),
		  "state":      reply.State,
	  })
   }

type bindHopReq struct {
	TokenID string `json:"token_id"`
	HopRef string `json:"hop_ref"`
	UserID string `json:"user_id"`
}

// bindHop POST /polhopgate/token/bind

   func (h *Handler) bindHop(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var req bindHopReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   reply, err := h.engine.BindHop(c.Request.Context(), BindRequest{
		   TokenID: req.TokenID,
		   HopRef: req.HopRef,
		  UserID: req.UserID,
	  })
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{
		   "token_id": reply.TokenID,
		  "policy_gen": reply.PolicyGen,
		   "state": reply.State,
	   })
  }

// ─── 租约接口 ─────────────────────────────────────────────────────────────

type acquireLeaseReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Holder string `json:"holder"`
}

// acquireLease POST /polhopgate/lease/acquire
  func (h *Handler) acquireLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var req acquireLeaseReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   reply, err := h.engine.AcquireLease(c.Request.Context(), AcquireLeaseRequest{
		  UserID: req.UserID,
		  Asset:  req.Asset,
		  Holder: req.Holder,
	  })
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{
		  "lease_id": reply.LeaseID,
		   "expires_at": reply.ExpiresAt.Format(time.RFC3339),
	   })
   }

type releaseLeaseReq struct {
	LeaseID      string `json:"lease_id"`
	CompletedGen int64  `json:"completed_gen"`
}

// releaseLease POST /polhopgate/lease/release
  func (h *Handler) releaseLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var req releaseLeaseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   if err := h.engine.ReleaseLease(c.Request.Context(), req.LeaseID, req.CompletedGen); err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"status": "released", "lease_id": req.LeaseID})
   }

// ─── 双读观测接口 ─────────────────────────────────────────────────────────

type recordDualReadReq struct {
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	BizKind string `json:"biz_kind"`
	PrimaryGen int64 `json:"primary_gen"`
	ReplicaGen int64 `json:"replica_gen"`
	TraceID string `json:"trace_id"`
}

// recordDualRead POST /polhopgate/dualread/record

  func (h *Handler) recordDualRead(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var req recordDualReadReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	  if err := h.engine.RecordDualRead(c.Request.Context(), DualReadRequest{
		  UserID: req.UserID,
		  Asset:      req.Asset,
		   BizKind:    req.BizKind,
		   PrimaryGen: req.PrimaryGen,
		   ReplicaGen: req.ReplicaGen,
		   TraceID:    req.TraceID,
	   }); err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"status": "recorded"})
   }

// ─── 漂移案例接口 ─────────────────────────────────────────────────────────
// getDriftCase GET /polhopgate/drift-case/:case_id

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
	   c.JSON(http.StatusOK,
		  buildDriftCaseResp(dc))
  }

// listDriftCases GET /polhopgate/drift-cases?state=&limit=
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
	  out := make([]gin.H,
		   0, len(cases))
	  for _, dc := range cases {
		  out = append(out,
			   buildDriftCaseResp(dc))
	  }
	   c.JSON(http.StatusOK, gin.H{"cases": out, "count": len(out)})
  }

type confirmDriftCaseReq struct {
	CaseID      string `json:"case_id"`
	ConfirmedBy string `json:"confirmed_by"`
	CloseReason string `json:"close_reason"`
}

// confirmDriftCase POST /polhopgate/drift-case/confirm
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
	Reason      string `json:"reason"`
}

// dismissDriftCase POST /polhopgate/drift-case/dismiss

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

// ─── 强制复验接口 ─────────────────────────────────────────────────────────

type forceReEvaluateReq struct {
	UserID string `json:"user_id"`
	Asset      string `json:"asset"`
	BizKind    string `json:"biz_kind"`
	CurrentGen int64  `json:"current_gen"`
	BlockReason string `json:"block_reason"`
}

// forceReEvaluate POST /polhopgate/force-re-evaluate

  func (h *Handler) forceReEvaluate(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var req forceReEvaluateReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  reply, err := h.engine.ForceReEvaluate(c.Request.Context(), ForceReEvaluateRequest{
		   UserID: req.UserID,
		   Asset: req.Asset,
		  BizKind:    req.BizKind,
		   CurrentGen: req.CurrentGen,
		   BlockReason: req.BlockReason,
	   })
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "blocked_count": reply.BlockedCount,
		  "skipped_count": reply.SkippedCount,
	   })
   }

// ─── DualEscalate 状态接口 ────────────────────────────────────────────────
// getEscalate GET /polhopgate/escalate?user_id=&asset=&biz_kind=
   func (h *Handler) getEscalate(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  userID := strings.TrimSpace(c.Query("user_id"))
	   asset := strings.TrimSpace(c.Query("asset"))
	  bizKind := strings.TrimSpace(c.Query("biz_kind"))
	  esc, err := h.engine.GetEscalate(c.Request.Context(), userID, asset, bizKind)
	   if err != nil {
		   c.JSON(http.StatusOK, gin.H{"active": false, "biz_kind": bizKind})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "active": true,
		   "escalate_id":       esc.ID,
		  "biz_kind":          esc.BizKind,
		  "threshold_count":   esc.ThresholdCount,
		   "max_diverge_delta": esc.MaxDivergeDelta,
		  "case_id": esc.CaseID,
		  "escalated_at": esc.EscalatedAt.Format(time.RFC3339),
	  })
  }

// ─── 辅助函数 ─────────────────────────────────────────────────────────────

   func buildDriftCaseResp(dc *PolDriftCase) gin.H {
	  resp := gin.H{
		   "case_id": dc.ID,
		   "user_id": dc.UserID,
		  "asset":    dc.Asset,
		  "biz_kind": dc.BizKind,
		  "trigger":     dc.Trigger,
		  "trigger_gen": dc.TriggerGen,
		  "replica_gen": dc.ReplicaGen,
		   "diverge_delta": dc.DivergeDelta,
		   "state": dc.State,
		  "proof_id":   dc.ProofID,
		   "blocked_at": dc.BlockedAt.Format(time.RFC3339),
		   "scan_note": dc.ScanNote,
	   }
	  if dc.ConfirmedAt != nil {
		  resp["confirmed_at"] = dc.ConfirmedAt.Format(time.RFC3339)
		   resp["confirmed_by"] = dc.ConfirmedBy
	   }
	   if dc.HopTokenID != "" {
		  resp["hop_token_id"] = dc.HopTokenID
	  }
	  return resp
  }
