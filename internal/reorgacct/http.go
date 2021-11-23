package reorgacct

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 处理器，持有 Service 引用。

type Handler struct {
	svc *Service
}

// NewHandler 构造 Handler；svc 不得为 nil。
  func NewHandler(svc *Service) *Handler {
	   return &Handler{svc: svc}
  }

// RegisterRoutes 将所有路由挂载到 gin.RouterGroup（调用方负责鉴权中间件）。

  func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	   rg.GET("/events", h.listEvents)
	  rg.GET("/events/:id", h.getEvent)
	  rg.POST("/events", h.ingestEvent)
	  rg.GET("/events/:id/impact", h.getImpact)
	   rg.POST("/events/:id/scan", h.triggerScan)
	   rg.POST("/events/:id/close", h.closeEvent)
  }

// ────────────────────────────────────────────────────────────────────────────
// GET /v1/reorgacct/events
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) listEvents(c *gin.Context) {
	   chain := c.Query("chain")
	   state := c.Query("state")
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	  offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	  events, err := h.svc.store.ListEvents(c.Request.Context(), chain, state, limit, offset)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"events": events, "limit": limit, "offset": offset})
   }

// ────────────────────────────────────────────────────────────────────────────
// GET /v1/reorgacct/events/:id
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) getEvent(c *gin.Context) {
	  id := c.Param("id")
	  ev, err := h.svc.store.GetEvent(c.Request.Context(), id)
	  if err != nil {
		   status := http.StatusInternalServerError
		  if err == ErrEventNotFound {
			  status = http.StatusNotFound
		   }
		   c.JSON(status, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, ev)
  }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgacct/events （手动摄取，ops/场景）
// ────────────────────────────────────────────────────────────────────────────

  func (h *Handler) ingestEvent(c *gin.Context) {
	  var req IngestReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		   return
	  }
	   ev,
		   err := h.svc.Ingest(c.Request.Context(), req)
	  if err == ErrDupEvent {
		   c.JSON(http.StatusConflict, gin.H{"event": ev, "note": "duplicate event, idem key matched"})
		  return
	  }
	   if err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusCreated, ev)
  }

// ────────────────────────────────────────────────────────────────────────────
// GET /v1/reorgacct/events/:id/impact
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) getImpact(c *gin.Context) {
	  id := c.Param("id")
	   ctx := c.Request.Context()
	  rows, err := h.svc.store.ListImpactRows(ctx, id)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	   summary := Summarize(id, rows)
	   c.JSON(http.StatusOK, gin.H{"summary": summary, "rows": rows})
  }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgacct/events/:id/scan
// ────────────────────────────────────────────────────────────────────────────

   func (h *Handler) triggerScan(c *gin.Context) {
	  id := c.Param("id")
	   ctx := c.Request.Context()
	  rows, err := h.svc.ScanEvent(ctx, id)
	  if err == ErrImpactSealed {
		  c.JSON(http.StatusConflict, gin.H{"error": "impact already sealed for this event"})
		   return
	  }
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	   summary := Summarize(id, rows)
	  c.JSON(http.StatusOK, gin.H{"summary": summary, "row_count": len(rows)})
  }

// ────────────────────────────────────────────────────────────────────────────
// POST /v1/reorgacct/events/:id/close
// ────────────────────────────────────────────────────────────────────────────

  func (h *Handler) closeEvent(c *gin.Context) {
	   id := c.Param("id")
	   var body struct {
		   Note string `json:"note"`
	   }
	   _ = c.ShouldBindJSON(&body)
	  err := h.svc.Close(c.Request.Context(), id)
	   if err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"event_id": id, "state": EventStateClosed, "note": body.Note})
   }
