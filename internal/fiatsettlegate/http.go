package fiatsettlegate

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/fiatrail"
)

// Handler ops 处理器。

type Handler struct {
	svc *Service
}

// NewHandler 创建处理器。
   func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register 注册路由。

   func (h *Handler) Register(rg *gin.RouterGroup) {
	  g := rg.Group("/fiat-settle-gate")
	   g.POST("/book-reserve", h.bookReserve)
	   g.POST("/authorize-lease", h.authorize)
	  g.POST("/settle-assert", h.settle)
	   g.POST("/refund-lease", h.refund)
	  g.POST("/retry-compensate", h.retry)
	   g.POST("/revoke", h.revoke)
	  g.POST("/worker/tick", h.tick)
	   g.GET("/lease", h.getLease)
	  g.GET("/view", h.view)
	  g.GET("/proofs", h.proofs)
	  g.GET("/reservations", h.reservations)
  }

   func (h *Handler) bookReserve(c *gin.Context) {
	  var req fiatrail.BookReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	  b,
		  res, err := h.svc.BookWithReserve(c.Request.Context(), req)
	  if err != nil {
		  writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"booking": b, "reservation": res})
   }

   func (h *Handler) authorize(c *gin.Context) {
	  var req AuthorizeLeaseRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	   lease, booking, err := h.svc.AuthorizeWithLease(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"lease": lease, "booking": booking})
   }

   func (h *Handler) settle(c *gin.Context) {
	   var req SettleAssertRequest
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  lease, err := h.svc.SettleAssert(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, lease)
  }

  func (h *Handler) refund(c *gin.Context) {
	  var req RefundWithLeaseRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	  rf, lease, err := h.svc.RefundWithLease(c.Request.Context(), req)
	  if err != nil {
		  writeErr(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"refund_lease": rf, "settle_lease": lease})
  }

  func (h *Handler) retry(c *gin.Context) {
	   var body struct {
		  LeaseID string `json:"fiat_settle_lease_id"`
		   Actor   string `json:"actor"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   lease, err := h.svc.RetryCompensate(c.Request.Context(), body.LeaseID, body.Actor)
	  if err != nil {
		  writeErr(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, lease)
  }

   func (h *Handler) revoke(c *gin.Context) {
	  var body struct {
		  LeaseID string `json:"fiat_settle_lease_id"`
		   Actor  string `json:"actor"`
		   Reason string `json:"reason"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  lease, err := h.svc.RevokeLease(c.Request.Context(), body.LeaseID, body.Actor, body.Reason)
	  if err != nil {
		  writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, lease)
  }

  func (h *Handler) tick(c *gin.Context) {
	   w := NewWorker(h.svc, DefaultWorkerConfig())
	  c.JSON(http.StatusOK,
		  w.Tick(c.Request.Context()))
  }

  func (h *Handler) getLease(c *gin.Context) {
	   id := c.Query("fiat_settle_lease_id")
	  lease, err := h.svc.GetLease(c.Request.Context(), id)
	  if err != nil {
		  writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, lease)
  }

   func (h *Handler) view(c *gin.Context) {
	  v, err := h.svc.View(c.Request.Context(), c.Query("booking_id"))
	   if err != nil {
		  writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, v)
  }

   func (h *Handler) proofs(c *gin.Context) {
	  leaseID := c.Query("fiat_settle_lease_id")
	  if leaseID != "" {
		   rows, err := h.svc.ExportProofs(c.Request.Context(), leaseID)
		   if err != nil {
			   writeErr(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"proofs": rows})
		   return
	   }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  rows, err := h.svc.store.ListProofs(c.Request.Context(), limit)
	   if err != nil {
		  writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": rows})
  }

  func (h *Handler) reservations(c *gin.Context) {
	   id := c.Query("booking_id")
	   r,
		  err := h.svc.store.GetReservationByBooking(c.Request.Context(), id)
	   if err != nil {
		  writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, r)
  }

  func writeErr(c *gin.Context, err error) {
	  code := http.StatusConflict
	   switch err {
	  case ErrLeaseNotFound,
		   ErrBooking, ErrReservationNotFound, ErrProofNotFound, ErrRefundLeaseNotFound:
		  code = http.StatusNotFound
	   case ErrActor,
		  ErrReason, ErrTTL:
		   code = http.StatusBadRequest
	   case ErrUnavailable, ErrLedger, ErrPSP:
		   code = http.StatusServiceUnavailable
	   }
	  c.JSON(code, gin.H{"error": err.Error()})
   }
