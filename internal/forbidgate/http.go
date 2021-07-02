// 功能：禁提闸 HTTP/ops API：禁提世代查询、漂移双人确认、证明导出、出金预检绑定管理。
// 挂载路径建议：/v1/forbid-gate/...
// 所有写操作要求 actor header（X-Actor），通过 authn 中间件鉴权。

// 作者：Cason
// 日期：2021.06.01

package forbidgate

import (
	"net/http"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 处理器。

type Handler struct {
	gate  *Gate
	drift *DriftService
}

// NewHandler 构造处理器。
   func NewHandler(gate *Gate, drift *DriftService) *Handler {
	   return &Handler{gate: gate, drift: drift}
   }

// Register 挂载路由。

  func (h *Handler) Register(rg *gin.RouterGroup) {
	  fg := rg.Group("/forbid-gate")
	  // 只读查询
	  fg.GET("/gen/:user_id", h.getGen)
	   fg.GET("/binds/:user_id", h.listBinds)
	   fg.GET("/drift-cases/:user_id", h.listDriftCases)
	  fg.GET("/drift-case/:case_id", h.getDriftCase)
	  fg.GET("/proof/export/:case_id", h.exportProof)
	  // 预检绑定操作
	   fg.POST("/binds/issue", h.issueBind)
	   fg.POST("/binds/consume", h.consumeBind)
	   // 漂移证明操作
	   fg.POST("/proof/submit", h.submitProof)
	  fg.POST("/proof/confirm", h.confirmProof)
  }

// ---- 只读查询 ----
// getGen GET /v1/forbid-gate/gen/:user_id
   func (h *Handler) getGen(c *gin.Context) {
	   userID := strings.TrimSpace(c.Param("user_id"))
	   if userID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		   return
	  }
	   view, err := h.gate.CurrentGeneration(c.Request.Context(), userID)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, view)
   }

// listBinds GET /v1/forbid-gate/binds/:user_id

  func (h *Handler) listBinds(c *gin.Context) {
	   userID := strings.TrimSpace(c.Param("user_id"))
	   if userID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		  return
	   }
	  if h.gate.repo == nil {
		   c.JSON(http.StatusServiceUnavailable, gin.H{"error": "repo unavailable"})
		   return
	   }
	  binds, err := h.gate.repo.ListOpenBindsByUser(c.Request.Context(), userID)
	  if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	   views := make([]BindView, 0, len(binds))
	   for _, b := range binds {
		   views = append(views, BindView{
			   ID:     b.ID,
			   UserID: b.UserID,
			  WithdrawRef: b.WithdrawRef,
			  Generation:  b.Generation,
			   State: b.State,
			  IssuedAt:  b.IssuedAt,
			  ExpiresAt: b.ExpiresAt,
		   })
	   }
	  c.JSON(http.StatusOK, gin.H{"binds": views})
   }

// listDriftCases GET /v1/forbid-gate/drift-cases/:user_id

  func (h *Handler) listDriftCases(c *gin.Context) {
	  userID := strings.TrimSpace(c.Param("user_id"))
	  if userID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		   return
	   }
	   cases, err := h.drift.ListOpenDriftCases(c.Request.Context(), userID)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	   views := make([]DriftCaseView, 0, len(cases))
	  for _, dc := range cases {
		  views = append(views, DriftCaseView{
			  ID: dc.ID,
			   UserID: dc.UserID,
			  WithdrawRef: dc.WithdrawRef,
			   Kind: dc.Kind,
			   State: dc.State,
			   DetectedAt: dc.DetectedAt,
			  ProvenAt:   dc.ProvenAt,
			   ClosedAt:   dc.ClosedAt,
			  ProofID:    dc.ProofID,
		  })
	  }
	   c.JSON(http.StatusOK, gin.H{"drift_cases": views})
   }

// getDriftCase GET /v1/forbid-gate/drift-case/:case_id
  func (h *Handler) getDriftCase(c *gin.Context) {
	   caseID := strings.TrimSpace(c.Param("case_id"))
	   if caseID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "case_id required"})
		  return
	  }
	   dc, err := h.drift.GetDriftCase(c.Request.Context(), caseID)
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, DriftCaseView{
		   ID: dc.ID,
		   UserID: dc.UserID,
		   WithdrawRef: dc.WithdrawRef,
		   Kind:        dc.Kind,
		   State:       dc.State,
		  DetectedAt: dc.DetectedAt,
		  ProvenAt:   dc.ProvenAt,
		  ClosedAt:   dc.ClosedAt,
		  ProofID: dc.ProofID,
	   })
  }

// exportProof GET /v1/forbid-gate/proof/export/:case_id

  func (h *Handler) exportProof(c *gin.Context) {
	   caseID := strings.TrimSpace(c.Param("case_id"))
	   if caseID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "case_id required"})
		   return
	  }
	   out, err := h.drift.ExportProof(c.Request.Context(), caseID)
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, out)
   }

// ---- 预检绑定操作 ----
// issueBindReq POST /v1/forbid-gate/binds/issue 请求体。
type issueBindReq struct {
	UserID string `json:"user_id"`
	WithdrawRef string `json:"withdraw_ref"`
	TTLMS       int64  `json:"ttl_ms,omitempty"`
}

// issueBind POST /v1/forbid-gate/binds/issue

  func (h *Handler) issueBind(c *gin.Context) {
	  var req issueBindReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   req.UserID = strings.TrimSpace(req.UserID)
	  if req.UserID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		  return
	  }
	   ttl := DefaultBindTTL
	  if req.TTLMS > 0 {
		  ttl = time.Duration(req.TTLMS) * time.Millisecond
	  }
	  bind, err := h.gate.IssueWdPrefightBind(c.Request.Context(), req.UserID, req.WithdrawRef, ttl)
	  if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, BindView{
		  ID:     bind.ID,
		   UserID: bind.UserID,
		   WithdrawRef: bind.WithdrawRef,
		  Generation:  bind.Generation,
		   State:     bind.State,
		   IssuedAt:  bind.IssuedAt,
		  ExpiresAt: bind.ExpiresAt,
	   })
  }

// consumeBindReq POST /v1/forbid-gate/binds/consume 请求体。
type consumeBindReq struct {
	BindID string `json:"bind_id"`
	UserID string `json:"user_id"`
}

// consumeBind POST /v1/forbid-gate/binds/consume

  func (h *Handler) consumeBind(c *gin.Context) {
	   var req consumeBindReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  if err := h.gate.CheckAndConsumeBind(c.Request.Context(), req.BindID, req.UserID); err != nil {
		   code := http.StatusInternalServerError
		   switch {
		  case strings.Contains(err.Error(), ErrBindRevoked.Error()):
			   code = http.StatusConflict
		  case strings.Contains(err.Error(),
			   ErrBindExpired.Error()):
			  code = http.StatusGone
		  case strings.Contains(err.Error(), ErrDriftWriteBlock.Error()):
			   code = http.StatusLocked
		  case strings.Contains(err.Error(), ErrBindNotFound.Error()):
			  code = http.StatusNotFound
		   }
		   c.JSON(code, gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"consumed": true})
   }

// ---- 漂移证明操作 ----
// submitProofReq POST /v1/forbid-gate/proof/submit 请求体。
type submitProofReq struct {
	CaseID string `json:"case_id"`
	Actor    string `json:"actor"`
	Summary  string `json:"summary"`
	Evidence string `json:"evidence"`
}

// submitProof POST /v1/forbid-gate/proof/submit
   func (h *Handler) submitProof(c *gin.Context) {
	   var req submitProofReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  if req.CaseID == "" || req.Actor == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "case_id and actor required"})
		  return
	  }
	   proof, err := h.drift.SubmitProof(c.Request.Context(), req.CaseID, req.Actor, req.Summary, req.Evidence)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proof_id": proof.ID, "submitted_at": proof.SubmittedAt})
  }

// confirmProofReq POST /v1/forbid-gate/proof/confirm 请求体。
type confirmProofReq struct {
	ProofID   string `json:"proof_id"`
	Confirmer string `json:"confirmer"`
}

// confirmProof POST /v1/forbid-gate/proof/confirm

   func (h *Handler) confirmProof(c *gin.Context) {
	   var req confirmProofReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	   if req.ProofID == "" || req.Confirmer == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "proof_id and confirmer required"})
		   return
	   }
	  if err := h.drift.ConfirmProof(c.Request.Context(), req.ProofID, req.Confirmer); err != nil {
		  code := http.StatusInternalServerError
		  if strings.Contains(err.Error(), ErrProofConfirmConflict.Error()) {
			  code = http.StatusConflict
		   }
		  c.JSON(code, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"confirmed": true})
  }
