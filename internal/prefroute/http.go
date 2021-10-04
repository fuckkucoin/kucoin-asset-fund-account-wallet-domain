package prefroute

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

// Handler prefroute HTTP 处理器。
type Handler struct {
	svc *Service
}

// NewHandler 构造 Handler。
   func NewHandler(svc *Service) *Handler {
	   return &Handler{svc: svc}
  }

// Register 注册所有路由到 gin RouterGroup。
   func (h *Handler) Register(g *gin.RouterGroup) {
	  // 偏好世代
	   g.GET("/users/:userID/generation", h.GetGeneration)
	   g.GET("/users/:userID/generations", h.ListGenerations)
	  // DefaultAccount 切换
	   g.PUT("/users/:userID/default-account", h.PutDefaultAccount)
	  g.GET("/users/:userID/default-account", h.GetDefaultAccount)
	  g.GET("/users/:userID/pending-switches", h.ListPendingSwitches)
	  // 租约
	   g.POST("/leases", h.IssueLease)
	   g.GET("/leases/:leaseID", h.GetLease)
	   g.POST("/leases/:leaseID/bind-snap", h.BindRouteSnap)
	   // 路由快照
	  g.GET("/snaps/:snapID", h.GetRouteSnap)
	  // 漂移
	   g.POST("/snaps/:snapID/confirm-credit", h.ConfirmCredit)
	   g.GET("/users/:userID/drift-cases", h.ListOpenDriftCases)
	   g.GET("/drift-cases/:caseID", h.GetDriftCase)
	   g.POST("/drift-cases/:caseID/proof", h.SubmitDriftProof)
	  g.POST("/drift-cases/:caseID/close", h.ConfirmDriftClose)
	   g.GET("/proofs/:proofID/export", h.ExportDriftProof)
	  // ops 监控
	   g.GET("/users/:userID/inflight-credits", h.ListInflightCredits)
	   g.GET("/users/:userID/active-leases", h.ListActiveLeases)
	  g.GET("/users/:userID/pref-write-blocked", h.IsPrefWriteBlocked)
  }

// ── 偏好世代 ─────────────────────────────────────────────────────────────────
// GetGeneration GET /users/:userID/generation
  func (h *Handler) GetGeneration(c *gin.Context) {
	  userID := c.Param("userID")
	   gen, err := h.svc.CurrentGeneration(c.Request.Context(), userID)
	  if err != nil {
		  c.JSON(httpErr(err))
		   return
	  }
	  c.JSON(http.StatusOK, gen)
   }

// ListGenerations GET /users/:userID/generations?limit=N

   func (h *Handler) ListGenerations(c *gin.Context) {
	   userID := c.Param("userID")
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	  gens, err := h.svc.ListGenerations(c.Request.Context(), userID, limit)
	   if err != nil {
		  c.JSON(httpErr(err))
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"generations": gens, "count": len(gens)})
   }

// ── DefaultAccount 切换 ────────────────────────────────────────────────────

type putDefaultAccountReq struct {
	Book        string `json:"book" binding:"required"`
	RequestedBy string `json:"requested_by"`
	ReqID       string `json:"req_id"`
	EnqueueOnInflight bool `json:"enqueue_on_inflight"`
}

// PutDefaultAccount PUT /users/:userID/default-account
   func (h *Handler) PutDefaultAccount(c *gin.Context) {
	   userID := c.Param("userID")
	   var req putDefaultAccountReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   book, err := fundpref.ParseBook(req.Book)
	  if err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   result, err := h.svc.PutDefaultAccount(c.Request.Context(), fundpref.PutDefaultAccountReq{
		  UserID:      userID,
		  Book:        book,
		  RequestedBy: req.RequestedBy,
		  ReqID: req.ReqID,
	  }, req.EnqueueOnInflight)
	  if err != nil {
		  c.JSON(httpErr(err))
		  return
	  }
	   c.JSON(http.StatusOK, result)
  }

// GetDefaultAccount GET /users/:userID/default-account

  func (h *Handler) GetDefaultAccount(c *gin.Context) {
	   userID := c.Param("userID")
	  pref, err := h.svc.GetDefaultPref(c.Request.Context(), userID)
	   if err != nil {
		  c.JSON(httpErr(err))
		  return
	  }
	  c.JSON(http.StatusOK, pref)
   }

// ListPendingSwitches GET /users/:userID/pending-switches
   func (h *Handler) ListPendingSwitches(c *gin.Context) {
	  userID := c.Param("userID")
	   items, err := h.svc.store.ListPendingSwitches(c.Request.Context(), userID, fundpref.PendingSwitchQueued)
	  if err != nil {
		  c.JSON(httpErr(err))
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"pending_switches": items, "count": len(items)})
   }

// ── 租约 ──────────────────────────────────────────────────────────────────────
	  })
	   if err != nil {
		  c.JSON(httpErr(err))
		  return
	   }
	  c.JSON(http.StatusCreated, lease)
   }

// GetLease GET /leases/:leaseID

   func (h *Handler) GetLease(c *gin.Context) {
	  leaseID := c.Param("leaseID")
	  lease, err := h.svc.GetLease(c.Request.Context(), leaseID)
	  if err != nil {
		   c.JSON(httpErr(err))
		   return
	   }
	  c.JSON(http.StatusOK, lease)
   }

type bindRouteSnapReq struct {
	CreditTxnID string `json:"credit_txn_id" binding:"required"`
	Asset    string `json:"asset" binding:"required"`
	Amount   string `json:"amount" binding:"required"`
	IssuedBy string `json:"issued_by"`
}

// BindRouteSnap POST /leases/:leaseID/bind-snap

  func (h *Handler) BindRouteSnap(c *gin.Context) {
	  leaseID := c.Param("leaseID")
	   var req bindRouteSnapReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   lease, snap, err := h.svc.BindRouteSnap(c.Request.Context(), BindRouteSnapReq{
		  LeaseID:     leaseID,
		   CreditTxnID: req.CreditTxnID,
		  Asset:       req.Asset,
		   Amount:      req.Amount,
		   IssuedBy:    req.IssuedBy,
	   })
	  if err != nil {
		   c.JSON(httpErr(err))
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"lease": lease, "route_snap": snap})
  }

// ── 路由快照 ──────────────────────────────────────────────────────────────────
// GetRouteSnap GET /snaps/:snapID

  func (h *Handler) GetRouteSnap(c *gin.Context) {
	   snapID := c.Param("snapID")
	  snap, err := h.svc.GetRouteSnap(c.Request.Context(), snapID)
	   if err != nil {
		  c.JSON(httpErr(err))
		   return
	   }
	  c.JSON(http.StatusOK, snap)
   }

type confirmCreditReq struct {
	CreditedBook string `json:"credited_book" binding:"required"`
	CreditTxnID string `json:"credit_txn_id"`
}

// ConfirmCredit POST /snaps/:snapID/confirm-credit
  func (h *Handler) ConfirmCredit(c *gin.Context) {
	   snapID := c.Param("snapID")
	  var req confirmCreditReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  book, err := fundpref.ParseBook(req.CreditedBook)
	  if err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   snap, dc, err := h.svc.ConfirmCredit(c.Request.Context(), ConfirmCreditReq{
		   RouteSnapID: snapID,
		  CreditedBook: book,
		   CreditTxnID:  req.CreditTxnID,
	   })
	  if err != nil {
		   c.JSON(httpErr(err))
		  return
	  }
	   resp := gin.H{"route_snap": snap}
	  if dc != nil {
		  resp["drift_case"] = dc
	  }
	   c.JSON(http.StatusOK, resp)
  }

// ── 漂移案例 ──────────────────────────────────────────────────────────────────
// ListOpenDriftCases GET /users/:userID/drift-cases
  func (h *Handler) ListOpenDriftCases(c *gin.Context) {
	   userID := c.Param("userID")
	   cases, err := h.svc.ListOpenDriftCases(c.Request.Context(), userID)
	  if err != nil {
		  c.JSON(httpErr(err))
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"drift_cases": cases, "count": len(cases)})
  }

// GetDriftCase GET /drift-cases/:caseID
   func (h *Handler) GetDriftCase(c *gin.Context) {
	  caseID := c.Param("caseID")
	  dc, err := h.svc.GetDriftCase(c.Request.Context(), caseID)
	   if err != nil {
		   c.JSON(httpErr(err))
		  return
	   }
	  c.JSON(http.StatusOK, dc)
  }

type submitDriftProofReq struct {
	SubmittedBy string `json:"submitted_by" binding:"required"`
	Summary string `json:"summary" binding:"required"`
	Evidence string `json:"evidence"`
}

// SubmitDriftProof POST /drift-cases/:caseID/proof
  func (h *Handler) SubmitDriftProof(c *gin.Context) {
	   caseID := c.Param("caseID")
	   var req submitDriftProofReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	  proof, err := h.svc.SubmitDriftProof(c.Request.Context(), SubmitDriftProofReq{
		  DriftCaseID: caseID,
		   SubmittedBy: req.SubmittedBy,
		   Summary:     req.Summary,
		   Evidence:    req.Evidence,
	   })
	   if err != nil {
		  c.JSON(httpErr(err))
		   return
	   }
	  c.JSON(http.StatusCreated,
		   proof)
   }

type confirmDriftCloseReq struct {
	ConfirmedBy string `json:"confirmed_by" binding:"required"`
}

// ConfirmDriftClose POST /drift-cases/:caseID/close
   func (h *Handler) ConfirmDriftClose(c *gin.Context) {
	   caseID := c.Param("caseID")
	   var req confirmDriftCloseReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	   dc, err := h.svc.ConfirmDriftClose(c.Request.Context(), caseID, req.ConfirmedBy)
	   if err != nil {
		   c.JSON(httpErr(err))
		   return
	  }
	  c.JSON(http.StatusOK, dc)
  }

// ExportDriftProof GET /proofs/:proofID/export

   func (h *Handler) ExportDriftProof(c *gin.Context) {
	   proofID := c.Param("proofID")
	  proof, err := h.svc.ExportDriftProof(c.Request.Context(), proofID)
	   if err != nil {
		   c.JSON(httpErr(err))
		   return
	   }
	   c.JSON(http.StatusOK, proof)
  }

// ── ops 监控 ──────────────────────────────────────────────────────────────────
// ListInflightCredits GET /users/:userID/inflight-credits
   func (h *Handler) ListInflightCredits(c *gin.Context) {
	   userID := c.Param("userID")
	  credits, err := h.svc.ActiveInflightCredits(c.Request.Context(), userID)
	  if err != nil {
		   c.JSON(httpErr(err))
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"inflight_credits": credits, "count": len(credits)})
   }

// ListActiveLeases GET /users/:userID/active-leases
   func (h *Handler) ListActiveLeases(c *gin.Context) {
	   userID := c.Param("userID")
	  leases, err := h.svc.ActiveLeases(c.Request.Context(), userID)
	   if err != nil {
		   c.JSON(httpErr(err))
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"leases": leases, "count": len(leases)})
   }

// IsPrefWriteBlocked GET /users/:userID/pref-write-blocked
  func (h *Handler) IsPrefWriteBlocked(c *gin.Context) {
	  userID := c.Param("userID")
	  blocked, caseID, err := h.svc.IsPrefWriteBlocked(c.Request.Context(), userID)
	  if err != nil {
		  c.JSON(httpErr(err))
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		   "blocked": blocked,
		   "case_id": caseID,
	   })
   }

// ── 错误映射 ──────────────────────────────────────────────────────────────────

  func httpErr(err error) (int, any) {
	  if err == nil {
		   return http.StatusOK, nil
	  }
	   switch {
	   case err == ErrNotFound || err == fundpref.ErrNotFound:
		   return http.StatusNotFound, gin.H{"error": err.Error()}
	   case err == ErrInflightCredit || err == fundpref.ErrInflightCredit:
		   return http.StatusConflict, gin.H{"error": err.Error(), "code": "inflight_credit"}
	   case err == ErrDriftUnproven || err == fundpref.ErrDriftUnproven:
		  return http.StatusConflict, gin.H{"error": err.Error(), "code": "drift_unproven"}
	   case err == ErrLeaseExpired || err == fundpref.ErrLeaseExpired:
		   return http.StatusConflict, gin.H{"error": err.Error(), "code": "lease_expired"}
	   case err == ErrLeaseInvalid || err == fundpref.ErrLeaseInvalid:
		  return http.StatusConflict, gin.H{"error": err.Error(), "code": "lease_invalid"}
	   case err == ErrGenerationMismatch || err == fundpref.ErrGenerationMismatch:
		  return http.StatusConflict, gin.H{"error": err.Error(), "code": "generation_mismatch"}
	  case err == ErrStaleFence || err == fundpref.ErrStaleFence:
		   return http.StatusConflict, gin.H{"error": err.Error(), "code": "stale_fence"}
	  case err == ErrInvalidInput || err == fundpref.ErrInvalidInput:
		  return http.StatusBadRequest, gin.H{"error": err.Error()}
	   default:
		  return http.StatusInternalServerError, gin.H{"error": err.Error()}
	  }
  }
