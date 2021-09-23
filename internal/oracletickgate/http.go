package oracletickgate

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// Handler ops 处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建处理器。

  func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register 注册路由。
  func (h *Handler) Register(rg *gin.RouterGroup) {
	   g := rg.Group("/oracle-tick-gate")
	  g.POST("/issue-lease", h.issue)
	   g.POST("/assert-fund-write", h.assertWrite)
	   g.POST("/revoke", h.revoke)
	  g.POST("/shadow", h.shadow)
	   g.POST("/probe", h.probe)
	  g.POST("/export-proof", h.exportProof)
	   g.POST("/worker/tick", h.tick)
	  g.GET("/lease", h.getLease)
	  g.GET("/view", h.view)
	  g.GET("/proofs", h.proofs)
	  g.GET("/ticks", h.ticks)
	  g.GET("/recheck", h.recheck)
  }

   func (h *Handler) issue(c *gin.Context) {
	  var req IssueTickLeaseRequest
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  lease,
		  err := h.svc.IssueTickLease(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, lease)
   }

  func (h *Handler) assertWrite(c *gin.Context) {
	  var req AssertFundWriteRequest
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   lease, rec, err := h.svc.AssertBeforeFundWrite(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"lease": lease, "write": rec})
  }

  func (h *Handler) revoke(c *gin.Context) {
	   var req RevokeLeaseRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	  lease, err := h.svc.RevokeLease(c.Request.Context(), req)
	  if err != nil {
		  writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, lease)
   }

   func (h *Handler) shadow(c *gin.Context) {
	  var req IssueTickLeaseRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   lease, err := h.svc.ShadowValuation(c.Request.Context(), req)
	   if err != nil {
		  writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, lease)
   }

   func (h *Handler) probe(c *gin.Context) {
	   var req IssueTickLeaseRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	   lease, err := h.svc.ProbeValuation(c.Request.Context(), req)
	  if err != nil {
		  writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, lease)
   }

   func (h *Handler) exportProof(c *gin.Context) {
	   var body struct {
		   ProofID string `json:"proof_id"`
	  }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   p, err := h.svc.ExportProof(c.Request.Context(), body.ProofID)
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, p)
   }

   func (h *Handler) tick(c *gin.Context) {
	  w := NewWorker(h.svc, DefaultWorkerConfig())
	  c.JSON(http.StatusOK, w.Tick(c.Request.Context()))
   }

   func (h *Handler) getLease(c *gin.Context) {
	  id := c.Query("tick_lease_id")
	   lease, err := h.svc.GetLease(c.Request.Context(), id)
	  if err != nil {
		  writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, lease)
  }

  func (h *Handler) view(c *gin.Context) {
	  id := c.Query("tick_lease_id")
	  v, err := h.svc.View(c.Request.Context(), id)
	   if err != nil {
		   writeErr(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, v)
  }

   func (h *Handler) proofs(c *gin.Context) {
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   rows, err := h.svc.ListProofs(c.Request.Context(), limit)
	  if err != nil {
		  writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": rows})
  }

   func (h *Handler) ticks(c *gin.Context) {
	  c.JSON(http.StatusOK, gin.H{"ticks": h.svc.Feed().List()})
   }

  func (h *Handler) recheck(c *gin.Context) {
	  id := c.Query("tick_lease_id")
	   rc := h.svc.RecheckTick(c.Request.Context(), id)
	   c.JSON(http.StatusOK, rc)
  }

   func writeErr(c *gin.Context, err error) {
	  code := http.StatusConflict
	   switch err {
	   case ErrLeaseNotFound, ErrProofNotFound, ErrNoLease, ErrTickMissing:
		  code = http.StatusNotFound
	  case ErrActor, ErrTTL, ErrValuation, ErrPair, ErrWriteKind, ErrMode:
		   code = http.StatusBadRequest
	   case ErrUnavailable:
		  code = http.StatusServiceUnavailable
	  }
	  c.JSON(code, gin.H{"error": err.Error(), "fail_closed": true})
   }
