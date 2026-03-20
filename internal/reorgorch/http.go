package reorgorch

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 处理器。
type Handler struct {
	orch *Orchestrator
	store *Store
}

// NewHandler 构造 Handler；orch 和 store 不得为 nil。

  func NewHandler(orch *Orchestrator, store *Store) *Handler {
	   return &Handler{orch: orch,
		  store: store}
  }

// RegisterRoutes 挂载所有路由。

   func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	   rg.GET("/batches", h.listBatches)
	   rg.GET("/batches/:id", h.getBatch)
	   rg.GET("/batches/:id/legs", h.listLegs)
	  rg.GET("/batches/:id/incidents", h.listIncidents)
	  rg.POST("/plan/:event_id", h.planByEvent)
	  rg.POST("/batches/:id/execute", h.executeBatch)
	   rg.POST("/batches/:id/abort", h.abortBatch)
	   rg.POST("/legs/:id/approve", h.approveLeg)
	   rg.POST("/legs/:id/skip", h.skipLeg)
   }

// ────────────────────────────────────────────────────────────────────────────
// GET /v1/reorgorch/batches
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) listBatches(c *gin.Context) {
	  eventID := c.Query("event_id")
	   state := c.Query("state")
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	  offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	   batches, err := h.store.ListBatches(c.Request.Context(), eventID, state, limit, offset)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"batches": batches, "limit": limit, "offset": offset})
  }

// ────────────────────────────────────────────────────────────────────────────
// GET /v1/reorgorch/batches/:id
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) getBatch(c *gin.Context) {
	  id := c.Param("id")
	   ctx := c.Request.Context()
	   batch, err := h.store.GetBatch(ctx, id)
	  if err != nil {
		   status := http.StatusInternalServerError
		   if err == ErrBatchNotFound {
			  status = http.StatusNotFound
		  }
		  c.JSON(status, gin.H{"error": err.Error()})
		   return
	   }
	   legs, _ := h.store.ListLegs(ctx, id)
	   c.JSON(http.StatusOK, gin.H{"batch": batch, "legs": legs})
   }

// ────────────────────────────────────────────────────────────────────────────
// GET /v1/reorgorch/batches/:id/legs
// ────────────────────────────────────────────────────────────────────────────

  func (h *Handler) listLegs(c *gin.Context) {
	   id := c.Param("id")
	  legs, err := h.store.ListLegs(c.Request.Context(), id)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"legs": legs})
  }

// ────────────────────────────────────────────────────────────────────────────
// GET /v1/reorgorch/batches/:id/incidents
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) listIncidents(c *gin.Context) {
	   id := c.Param("id")
	   tickets, err := h.store.ListIncidents(c.Request.Context(), id)
	  if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"incidents": tickets})
  }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgorch/plan/:event_id
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) planByEvent(c *gin.Context) {
	   eventID := c.Param("event_id")
	   batch, res, err := h.orch.Plan(c.Request.Context(), eventID)
	   if err == ErrGuardConflict {
		   c.JSON(http.StatusConflict, gin.H{"batch": batch, "plan_result": res, "note": "already planned"})
		   return
	   }
	  if err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusCreated, gin.H{"batch": batch, "plan_result": res})
  }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgorch/batches/:id/execute
// ────────────────────────────────────────────────────────────────────────────

  func (h *Handler) executeBatch(c *gin.Context) {
	   id := c.Param("id")
	  res, err := h.orch.Execute(c.Request.Context(), id)
	  if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "partial": res})
		   return
	   }
	  c.JSON(http.StatusOK, res)
   }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgorch/batches/:id/abort
// ────────────────────────────────────────────────────────────────────────────

  func (h *Handler) abortBatch(c *gin.Context) {
	   id := c.Param("id")
	   var body struct {
		   Reason string `json:"reason"`
	   }
	   _ = c.ShouldBindJSON(&body)
	  if err := h.orch.AbortBatch(c.Request.Context(), id, body.Reason); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"batch_id": id, "state": BatchStateAborted})
  }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgorch/legs/:id/approve
// ────────────────────────────────────────────────────────────────────────────

  func (h *Handler) approveLeg(c *gin.Context) {
	  id := c.Param("id")
	   var body struct {
		   Note string `json:"note"`
	  }
	  _ = c.ShouldBindJSON(&body)
	   res, err := h.orch.ApproveLeg(c.Request.Context(), id, body.Note)
	  if err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, res)
  }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgorch/legs/:id/skip
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) skipLeg(c *gin.Context) {
	   id := c.Param("id")
	   var body struct {
		  Reason string `json:"reason"`
	  }
	   _ = c.ShouldBindJSON(&body)
	   legFSM := NewLegFSM(h.store)
	   if err := legFSM.Skip(c.Request.Context(), id, body.Reason); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"leg_id": id, "state": LegStateSkipped})
  }
