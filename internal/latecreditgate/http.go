package latecreditgate

import (
	"errors"
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// Handler ops 处理器。

type Handler struct {
	svc *Service
}

// NewHandler 创建处理器。
   func NewHandler(svc *Service) *Handler {
	   return &Handler{svc: svc}
   }

// Register 注册路由。
   func (h *Handler) Register(rg *gin.RouterGroup) {
	  g := rg.Group("/late-credit-gate")
	  g.POST("/occupy", h.occupy)
	   g.POST("/check-credit", h.checkCredit)
	  g.POST("/auto-credit", h.autoCredit)
	   g.POST("/guard-write", h.guardWrite)
	  g.POST("/ack-race", h.ackRace)
	  g.GET("/races", h.listRaces)
	  g.GET("/proofs", h.listProofs)
	  g.GET("/gen", h.getGen)
	  g.GET("/view", h.view)
   }

  func (h *Handler) occupy(c *gin.Context) {
	   var req OccupyRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	   g, err := h.svc.OccupyWithGen(c.Request.Context(), req)
	  if err != nil {
		  writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, g)
  }

  func (h *Handler) checkCredit(c *gin.Context) {
	   var req CreditRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   g,
		  err := h.svc.CheckCreditDurable(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, g)
   }

   func (h *Handler) autoCredit(c *gin.Context) {
	   var req CreditRequest
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   g, err := h.svc.AutoCredit(c.Request.Context(), req)
	   if err != nil {
		  writeErr(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, g)
  }

  func (h *Handler) guardWrite(c *gin.Context) {
	  var req WriteRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	  g, err := h.svc.GuardWrite(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, g)
   }

   func (h *Handler) ackRace(c *gin.Context) {
	   var req ResolveRaceRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   ev, err := h.svc.AckRace(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, ev)
   }

  func (h *Handler) listRaces(c *gin.Context) {
	   limit := queryInt(c, "limit", 200)
	   evs, err := h.svc.DrainOpenRaces(c.Request.Context(), limit)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"races": evs, "count": len(evs)})
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

  func (h *Handler) getGen(c *gin.Context) {
	   key := OutputKey{
		   Chain: c.Query("chain"),
		  TxHash:      c.Query("tx_hash"),
		   OutputIndex: queryInt(c, "output_index", 0),
	  }
	  g, err := h.svc.GetGeneration(c.Request.Context(), key, FenceToken{}, true)
	  if err != nil {
		  writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, g)
  }

  func (h *Handler) view(c *gin.Context) {
	  key := OutputKey{
		   Chain:  c.Query("chain"),
		   TxHash: c.Query("tx_hash"),
		  OutputIndex: queryInt(c, "output_index", 0),
	   }
	   v,
		   err := h.svc.View(c.Request.Context(), key)
	  if err != nil {
		   writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, v)
   }

   func writeErr(c *gin.Context, err error) {
	   code := http.StatusInternalServerError
	  switch {
	  case errors.Is(err, ErrFenceRequired), errors.Is(err, ErrFenceEpochZero), errors.Is(err, ErrFenceTokenEmpty):
		  code = http.StatusForbidden
	  case errors.Is(err, ErrCreditBlocked), errors.Is(err, ErrOccupyConflict), errors.Is(err, ErrAlreadyCredited),
		  errors.Is(err, ErrAlreadyReclaimed), errors.Is(err, ErrRaceStillOpen), errors.Is(err, ErrGenMismatch),
		  errors.Is(err,
			  ErrGenRollback), errors.Is(err, ErrStateInvalid):
		  code = http.StatusConflict
	   case errors.Is(err, ErrGenNotFound), errors.Is(err, ErrRaceEventNotFound), errors.Is(err, ErrProofNotFound):
		   code = http.StatusNotFound
	   case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrOutputKeyEmpty), errors.Is(err, ErrActorEmpty):
		   code = http.StatusBadRequest
	   }
	  c.JSON(code, gin.H{"error": err.Error()})
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
