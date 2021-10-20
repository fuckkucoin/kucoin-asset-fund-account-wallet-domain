// HTTP / ops API。
// 路由挂载于 /ops/recycle-epoch-gate/：
// POST /mark
// POST /ready
// POST /assign
// POST /observe-credit
// POST /retire
// POST /ack-proof
// GET /proofs
// GET /epoch
// GET /view

// Author: Cason
// Date: 2021.10.19

package recycleepoch

import (
	"errors"
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
}

  func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

   func (h *Handler) Register(rg *gin.RouterGroup) {
	  g := rg.Group("/recycle-epoch-gate")
	   g.POST("/mark", h.mark)
	  g.POST("/ready", h.ready)
	   g.POST("/assign", h.assign)
	  g.POST("/observe-credit", h.observe)
	  g.POST("/retire", h.retire)
	  g.POST("/ack-proof", h.ack)
	   g.GET("/proofs", h.listProofs)
	   g.GET("/epoch", h.getEpoch)
	  g.GET("/view", h.view)
	  g.GET("/generation", h.generation)
  }

   func (h *Handler) mark(c *gin.Context) {
	   var req MarkRequest
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   e, err := h.svc.MarkWithEpoch(c.Request.Context(), req)
	   if err != nil {
		  writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, e)
  }

   func (h *Handler) ready(c *gin.Context) {
	   var req ReadyRequest
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	  e,
		   err := h.svc.ReadyAssert(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, e)
  }

  func (h *Handler) assign(c *gin.Context) {
	   var req AssignRequest
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	  e, b, err := h.svc.AssignUser(c.Request.Context(), req)
	   if err != nil {
		  writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"epoch": e, "bind": b})
   }

   func (h *Handler) observe(c *gin.Context) {
	   var req ObserveCreditRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   rec, err := h.svc.ObserveCreditAssertEpoch(c.Request.Context(), req)
	  if err != nil {
		   if rec != nil {
			  c.JSON(statusOf(err), gin.H{"error": err.Error(), "observe": rec, "credit_posted": rec.CreditPosted()})
			  return
		  }
		   writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"observe": rec, "credit_posted": rec.CreditPosted()})
  }

  func (h *Handler) retire(c *gin.Context) {
	   var req RetireRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   e, err := h.svc.Retire(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, e)
  }

   func (h *Handler) ack(c *gin.Context) {
	  var req AckProofRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  p, err := h.svc.AckProof(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, p)
   }

   func (h *Handler) listProofs(c *gin.Context) {
	  limit := queryInt(c, "limit", 200)
	   ps, err := h.svc.ExportProofs(c.Request.Context(), limit)
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": ps, "count": len(ps)})
   }

  func (h *Handler) getEpoch(c *gin.Context) {
	  e, err := h.svc.GetEpoch(c.Request.Context(), c.Query("chain"), c.Query("address"))
	   if err != nil {
		   writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, e)
   }

  func (h *Handler) view(c *gin.Context) {
	   v, err := h.svc.View(c.Request.Context(), c.Query("chain"), c.Query("address"))
	   if err != nil {
		   writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, v)
   }

  func (h *Handler) generation(c *gin.Context) {
	  g, err := h.svc.CurrentGeneration(c.Request.Context(), c.Query("chain"), c.Query("address"))
	   if err != nil {
		   writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"generation": g})
   }

  func writeErr(c *gin.Context, err error) {
	  c.JSON(statusOf(err), gin.H{"error": err.Error()})
   }

   func statusOf(err error) int {
	   switch {
	  case errors.Is(err, ErrCoolNotElapsed), errors.Is(err, ErrForcedReuse),
		   errors.Is(err, ErrWrongUserCredit), errors.Is(err, ErrCreditBlocked),
		   errors.Is(err, ErrQuarantined), errors.Is(err, ErrEpochStale),
		   errors.Is(err, ErrEpochMissing), errors.Is(err, ErrBindRevoked),
		   errors.Is(err, ErrNotReady), errors.Is(err, ErrAlreadyAssigned),
		  errors.Is(err, ErrEpochMismatch), errors.Is(err, ErrEpochRollback),
		   errors.Is(err, ErrStateInvalid), errors.Is(err, ErrEpochRetired),
		  errors.Is(err, ErrObserveIdemConflict), errors.Is(err, ErrProofStillOpen):
		  return http.StatusConflict
	   case errors.Is(err, ErrEpochNotFound), errors.Is(err, ErrProofNotFound),
		   errors.Is(err, ErrBindNotFound):
		  return http.StatusNotFound
	   case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrActorEmpty),
		  errors.Is(err,
			  ErrAddressEmpty), errors.Is(err, ErrUserEmpty),
		  errors.Is(err,
			   ErrIdemEmpty), errors.Is(err, ErrAmountInvalid),
		   errors.Is(err, ErrTxHashEmpty):
		  return http.StatusBadRequest
	  default:
		   return http.StatusInternalServerError
	   }
   }

  func queryInt(c *gin.Context, key string, def int) int {
	  v := c.Query(key)
	  if v == "" {
		   return def
	   }
	   n, err := strconv.Atoi(v)
	  if err != nil {
		  return def
	   }
	  return n
  }
