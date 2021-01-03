package holdlayerapi

import (
	"net/http"
	"os"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
	"account-assets.kucoin.net/funding/internal/holdrel"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handler 分层冻结 HTTP 处理器集合（历史 holdcat / holdrel 面）。
type Handler struct {
	tokens *holdcat.TokenBook
	protocol *holdrel.Protocol
	gaps     *holdrel.GapStore
}

// NewHandler 构建历史面 Handler。
   func NewHandler(tokens *holdcat.TokenBook, protocol *holdrel.Protocol, gaps *holdrel.GapStore) *Handler {
	   return &Handler{tokens: tokens,
		  protocol: protocol, gaps: gaps}
   }

// Register 注册历史面路由到 gin 路由组（与包级 Register(v1, rt) 并存）。
   func (h *Handler) Register(rg *gin.RouterGroup) {
	   rg.POST("/hold-tokens", h.placeToken)
	   rg.POST("/hold-tokens/:id/release", h.releaseToken)
	  rg.GET("/hold-tokens/:id", h.getToken)
	   rg.GET("/hold-tokens", h.listTokens)
	  rg.GET("/release-proofs/:ref", h.getProof)
	   rg.GET("/hold-gaps", h.listGaps)
	  rg.POST("/hold-gaps/:id/ack", h.ackGap)
	   rg.POST("/hold-gaps/:id/close", h.closeGap)
	  rg.GET("/hold-layers/matrix", h.getMatrix)
  }

// ---- 请求/响应体 ----

type placeTokenReq struct {
	UserID string `json:"user_id" binding:"required"`
	Asset  string `json:"asset" binding:"required"`
	Book string `json:"book"`
	Amount string `json:"amount" binding:"required"`
	Layer string `json:"layer" binding:"required"`
	ReasonCode string `json:"reason_code" binding:"required"`
	LedgerRef  string `json:"ledger_ref"`
	IdemKey string `json:"idem_key" binding:"required"`
	ApprovalRef string `json:"approval_ref"`
	Note        string `json:"note"`
}

type releaseTokenReq struct {
	CallerLayer string `json:"caller_layer" binding:"required"`
	ReasonCode  string `json:"reason_code" binding:"required"`
	ApprovalRef string `json:"approval_ref"`
	IdemKey   string `json:"idem_key" binding:"required"`
	Requester string `json:"requester" binding:"required"`
	Note      string `json:"note"`
}

type ackCloseReq struct {
	Operator string `json:"operator" binding:"required"`
	Note string `json:"note"`
}

// ---- 处理器实现 ----

   func (h *Handler) placeToken(c *gin.Context) {
	  var req placeTokenReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   layer, err := holdcat.ParseLayer(req.Layer)
	   if err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid layer: " + req.Layer})
		  return
	  }
	  tok, err := h.tokens.Place(
		  c.Request.Context(),
		   req.UserID, req.Asset, req.Book, req.Amount,
		   layer,
		   holdcat.ReasonCode(req.ReasonCode),
		  req.LedgerRef, req.IdemKey, req.ApprovalRef, req.Note,
	  )
	   if err != nil {
		  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, tok)
   }

  func (h *Handler) releaseToken(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("id"))
	  if id == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "token id required"})
		   return
	  }
	  var req releaseTokenReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   proof, err := h.protocol.RequestRelease(c.Request.Context(), holdrel.ReleaseReq{
		   TokenID:     id,
		  CallerLayer: req.CallerLayer,
		   ReasonCode: req.ReasonCode,
		   ApprovalRef: req.ApprovalRef,
		   IdemKey:   req.IdemKey,
		  Requester: req.Requester,
		  Note: req.Note,
		   At:   time.Now().UTC(),
	   })
	   if err != nil {
		  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, proof)
   }

  func (h *Handler) getToken(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("id"))
	   tok, err := h.tokens.Get(c.Request.Context(), "", id)
	   if err != nil {
		  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, tok)
  }

   func (h *Handler) listTokens(c *gin.Context) {
	   userID := c.Query("user_id")
	   activeOnly := c.Query("active") == "1" || c.Query("active") == "true"
	   toks, err := h.tokens.List(c.Request.Context(), userID, activeOnly)
	  if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"tokens": toks, "count": len(toks)})
   }

  func (h *Handler) getProof(c *gin.Context) {
	   ref := strings.TrimSpace(c.Param("ref"))
	  proof, err := h.protocol.QueryProof(c.Request.Context(), ref)
	  if err != nil {
		   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, proof)
  }

   func (h *Handler) listGaps(c *gin.Context) {
	   userID := c.Query("user_id")
	  stateStr := c.Query("state")
	  gaps, err := h.gaps.List(c.Request.Context(), userID, holdrel.GapState(stateStr), 200)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"gaps": gaps, "count": len(gaps)})
   }

  func (h *Handler) ackGap(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("id"))
	  var req ackCloseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   gap, err := h.gaps.Ack(c.Request.Context(), id, req.Operator)
	  if err != nil {
		   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gap)
   }

  func (h *Handler) closeGap(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("id"))
	   var req ackCloseReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   gap, err := h.gaps.Close(c.Request.Context(), id, req.Operator, req.Note)
	   if err != nil {
		  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gap)
  }

   func (h *Handler) getMatrix(c *gin.Context) {
	  matrix := holdcat.BuildPriorityMatrix()
	   c.JSON(http.StatusOK, gin.H{"matrix": matrix})
  }

// httpStatus 将领域错误映射到 HTTP 状态码。

   func httpStatus(err error) int {
	  switch {
	   case err == nil:
		  return http.StatusOK
	   case isNotFound(err):
		  return http.StatusNotFound
	   case isPermission(err):
		   return http.StatusForbidden
	  case isConflict(err):
		   return http.StatusConflict
	   case isValidation(err):
		   return http.StatusBadRequest
	  default:
		   return http.StatusInternalServerError
	   }
  }

  func isNotFound(err error) bool {
	   s := err.Error()
	   return strings.Contains(s, "not found") || strings.Contains(s, "ErrGone")
   }

   func isPermission(err error) bool {
	  s := err.Error()
	  return strings.Contains(s, "cannot release") || strings.Contains(s, "insufficient priority") ||
		  strings.Contains(s, "higher") || strings.Contains(s, "denied")
  }

  func isConflict(err error) bool {
	  return strings.Contains(err.Error(), "already released")
   }

   func isValidation(err error) bool {
	  s := err.Error()
	   return strings.Contains(s, "required") || strings.Contains(s, "invalid") ||
		   strings.Contains(s, "mismatch") || strings.Contains(s, "approval_ref")
   }

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   secret := strings.TrimSpace(os.Getenv("HOLDOPS_GATE_HMAC"))
	   if secret == "" && rt != nil && rt.Mem {
		   secret = strings.TrimSpace(os.Getenv("HOLDOPS_GATE_HMAC_MEM"))
	  }
	  svc := New(holdopsgate.NewService(secret, holdopsgate.NewMemStore()))
	  registerRoutes(v1, svc)
   }

   func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	   g := v1.Group("/holdlayerapi")
	  g.POST("/place", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body PlaceBody
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.Place(c.Request.Context(), body)
		  writeOps(c, got, err)
	   })
	  g.POST("/release", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body ReleaseBody
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.Release(c.Request.Context(), body)
		  writeOps(c, got, err)
	  })
	  g.POST("/gap/ack", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body GapBody
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got,
			  err := svc.Ack(c.Request.Context(), body)
		  writeOps(c, got, err)
	  })
	   g.POST("/gap/close", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body GapBody
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Close(c.Request.Context(), body)
		  writeOps(c, got, err)
	  })
	   g.GET("/generation", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   got, err := svc.gate.GetGeneration(c.Request.Context())
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, got)
	  })
	  g.GET("/proof/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   got, err := svc.gate.GetProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, got)
	   })
  }

  func writeOps(c *gin.Context, got OpsReply, err error) {
	   if err != nil {
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "lease": got.Lease, "proof": got.Proof, "hold_ops_gen": got.Gen})
		   return
	   }
	  c.JSON(http.StatusOK, got)
  }
