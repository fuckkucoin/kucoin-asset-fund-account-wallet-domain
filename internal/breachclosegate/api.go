package breachclosegate

import (
	"errors"
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// Handler ops API handler。
type Handler struct {
	svc *Service
	scanBook ScanBook
}

// NewHandler 创建 Handler。scanBook 可为 nil（POST /scan 时将返回 501）。

  func NewHandler(svc *Service, scanBook ScanBook) *Handler {
	  return &Handler{svc: svc, scanBook: scanBook}
   }

// Register 将路由注册到 gin.RouterGroup。

   func (h *Handler) Register(rg *gin.RouterGroup) {
	  g := rg.Group("/breach-close-gate")
	   g.POST("/scan", h.triggerScan)
	  g.GET("/breaches", h.listBreaches)
	   g.GET("/breaches/:id", h.getBreach)
	  g.GET("/breaches/case/:case_id", h.getBreachByCase)
	  g.GET("/breaches/movement/:movement_id", h.getBreachByMovement)
	  g.POST("/try-allow-check", h.tryAllowCheck)
	   g.POST("/submit-precheck", h.submitPrecheck)
	   g.POST("/clearances", h.proposeClearance)
	  g.GET("/clearances", h.listClearances)
	  g.GET("/clearances/:id", h.getClearance)
	   g.PUT("/clearances/:id/approve", h.approveClearance)
	   g.PUT("/clearances/:id/reject", h.rejectClearance)
	  g.GET("/proofs", h.listProofs)
	  g.GET("/proofs/lease/:lease_id", h.listProofsByLease)
   }

// ── Scan ─────────────────────────────────────────────────────────────────────

   func (h *Handler) triggerScan(c *gin.Context) {
	  if h.scanBook == nil {
		   c.JSON(http.StatusNotImplemented, gin.H{"error": "scan book not configured"})
		  return
	  }
	  refs := h.scanBook.ScanRefs()
	   created := 0
	  errors_ := 0
	  for _, ref := range refs {
		  if _, err := h.svc.ScanAndBlock(ref); err != nil {
			   errors_++
		   } else {
			  created++
		  }
	   }
	  c.JSON(http.StatusOK, gin.H{
		  "scanned": len(refs),
		   "created": created,
		   "errors": errors_,
	   })
  }

// ── BreachLease 查询 ──────────────────────────────────────────────────────────

  func (h *Handler) listBreaches(c *gin.Context) {
	   limit := queryInt(c, "limit", 50)
	  leases := h.svc.ListActiveLeases(limit)
	   c.JSON(http.StatusOK, gin.H{"breaches": leases, "count": len(leases)})
  }

  func (h *Handler) getBreach(c *gin.Context) {
	  id := c.Param("id")
	   lease, err := h.svc.GetLease(id)
	   if err != nil {
		   if errors.Is(err, ErrLeaseNotFound) {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   } else {
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  }
		   return
	  }
	   c.JSON(http.StatusOK, lease)
  }

   func (h *Handler) getBreachByCase(c *gin.Context) {
	   caseID := c.Param("case_id")
	   lease, err := h.svc.GetActiveByCase(caseID)
	   if err != nil {
		  if errors.Is(err, ErrNoActiveBreach) {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  } else {
			  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  }
		   return
	   }
	   c.JSON(http.StatusOK, lease)
   }

   func (h *Handler) getBreachByMovement(c *gin.Context) {
	   movementID := c.Param("movement_id")
	  lease, err := h.svc.GetActiveByMovement(movementID)
	  if err != nil {
		   if errors.Is(err, ErrNoActiveBreach) {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   } else {
			  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   }
		   return
	   }
	  c.JSON(http.StatusOK, lease)
  }

// ── 预检 ─────────────────────────────────────────────────────────────────────

type tryAllowCheckReq struct {
	CaseID string `json:"case_id" binding:"required"`
	Actor  string `json:"actor"   binding:"required"`
}

  func (h *Handler) tryAllowCheck(c *gin.Context) {
	  var req tryAllowCheckReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  res, err := h.svc.TryAllowCheck(req.CaseID, req.Actor)
	  if err != nil && errors.Is(err, ErrBreachBlock) {
		  metrics.BreachAllowBlocked.Inc()
		  c.JSON(http.StatusForbidden, gin.H{
			   "allowed": false,
			  "error": err.Error(),
			   "lease": res.Lease,
			  "proof": res.Proof,
		  })
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"allowed": true})
  }

type submitPrecheckReq struct {
	MovementID string `json:"movement_id" binding:"required"`
	Actor      string `json:"actor"       binding:"required"`
}

  func (h *Handler) submitPrecheck(c *gin.Context) {
	  var req submitPrecheckReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   res,
		  err := h.svc.SubmitPrecheck(req.MovementID, req.Actor)
	   if err != nil && errors.Is(err, ErrBreachBlock) {
		   metrics.BreachAllowBlocked.Inc()
		   c.JSON(http.StatusForbidden, gin.H{
			  "allowed": false,
			   "error":   err.Error(),
			  "lease":   res.Lease,
			   "proof": res.Proof,
		   })
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"allowed": true})
  }

// ── 四眼清障 ─────────────────────────────────────────────────────────────────

type proposeClearanceReq struct {
	BreachLeaseID string `json:"breach_lease_id" binding:"required"`
	Proposer string `json:"proposer"        binding:"required"`
	Reason   string `json:"reason"          binding:"required"`
}

  func (h *Handler) proposeClearance(c *gin.Context) {
	  var req proposeClearanceReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  clr,
		   err := h.svc.ProposeBreachClearance(req.BreachLeaseID, req.Proposer, req.Reason)
	  if err != nil {
		   code := http.StatusInternalServerError
		  if errors.Is(err, ErrLeaseNotFound) {
			   code = http.StatusNotFound
		   } else if errors.Is(err, ErrClearanceMakerCheck) || errors.Is(err, ErrAlreadyCleared) || errors.Is(err, ErrAlreadyExpiredCase) || errors.Is(err, ErrDuplicateBreach) {
			   code = http.StatusConflict
		   }
		   c.JSON(code, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusCreated, clr)
  }

   func (h *Handler) listClearances(c *gin.Context) {
	   limit := queryInt(c, "limit", 50)
	  clears := h.svc.ListClearances(limit)
	   c.JSON(http.StatusOK, gin.H{"clearances": clears, "count": len(clears)})
   }

  func (h *Handler) getClearance(c *gin.Context) {
	  id := c.Param("id")
	  clr, err := h.svc.store.GetClearanceByID(id)
	  if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, clr)
   }

type approveReq struct {
	Approver string `json:"approver" binding:"required"`
}

  func (h *Handler) approveClearance(c *gin.Context) {
	   id := c.Param("id")
	   var req approveReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  clr, err := h.svc.ApproveBreachClearance(id, req.Approver)
	  if err != nil && !errors.Is(err, ErrClearanceNoAutoAllow) {
		  code := http.StatusInternalServerError
		  if errors.Is(err, ErrClearanceMakerCheck) || errors.Is(err, ErrClearanceNotPending) {
			  code = http.StatusConflict
		  } else if errors.Is(err, ErrClearanceNotFound) || errors.Is(err, ErrLeaseNotFound) {
			   code = http.StatusNotFound
		   }
		   c.JSON(code, gin.H{"error": err.Error()})
		   return
	  }
	   // ErrClearanceNoAutoAllow 是成功语义的一部分，体现在响应中。
	  c.JSON(http.StatusOK, gin.H{
		   "clearance":     clr,
		   "no_auto_allow": true,
		  "message":       "clearance approved; withdraw must re-enter compliance path",
	  })
  }

type rejectReq struct {
	Rejecter string `json:"rejecter" binding:"required"`
	Reason string `json:"reason"`
}

   func (h *Handler) rejectClearance(c *gin.Context) {
	   id := c.Param("id")
	   var req rejectReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   clr, err := h.svc.RejectBreachClearance(id, req.Rejecter, req.Reason)
	   if err != nil {
		  code := http.StatusInternalServerError
		   if errors.Is(err, ErrClearanceMakerCheck) || errors.Is(err, ErrClearanceNotPending) {
			  code = http.StatusConflict
		  } else if errors.Is(err, ErrClearanceNotFound) {
			  code = http.StatusNotFound
		   }
		  c.JSON(code, gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"clearance": clr, "breach_still_blocking": true})
   }

// ── 证明导出 ─────────────────────────────────────────────────────────────────

  func (h *Handler) listProofs(c *gin.Context) {
	  limit := queryInt(c, "limit", 200)
	   proofs := h.svc.ExportProofs(limit)
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
   }

   func (h *Handler) listProofsByLease(c *gin.Context) {
	  leaseID := c.Param("lease_id")
	  limit := queryInt(c, "limit", 50)
	  proofs := h.svc.ExportProofsByLease(leaseID,
		   limit)
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
   }

// ── 工具函数 ─────────────────────────────────────────────────────────────────

   func queryInt(c *gin.Context, key string, def int) int {
	  v := c.Query(key)
	   if v == "" {
		   return def
	  }
	  n, err := strconv.Atoi(v)
	   if err != nil || n <= 0 {
		   return def
	  }
	  return n
   }
