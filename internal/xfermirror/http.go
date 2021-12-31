// @FileName: http.go
// @Summary: xfermirror 包内 http 实现。
// 对手方镜像读模型 HTTP API。
// 路由（挂载到 v1 路由组下的 /xfermirror 前缀）：
// GET /xfermirror/views 列出镜像（按用户/资产/状态过滤）
// GET /xfermirror/views/:hop_id 查询单个镜像
// GET /xfermirror/views/:hop_id/events 查询镜像审计事件
// GET /xfermirror/views/:hop_id/align 检查与发起方的状态一致
// POST /xfermirror/views/:hop_id/manual 人工强制推进到 manual（ops）
// POST /xfermirror/views/:hop_id/materialize 立即物化（ops，触发泵单次执行）
// @Author: Bronson
// @Created: 2021.12.30

package xfermirror

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 处理器。

type Handler struct {
	machine *Machine
	pump    *Pump
}

// NewHandler 构造处理器。
  func NewHandler(machine *Machine, pump *Pump) *Handler {
	   return &Handler{machine: machine,
		   pump: pump}
  }

// Register 注册路由。
   func (h *Handler) Register(rg *gin.RouterGroup) {
	   g := rg.Group("/xfermirror")
	  g.GET("/views", h.listViews)
	   g.GET("/views/:hop_id", h.getView)
	   g.GET("/views/:hop_id/events", h.getEvents)
	   g.GET("/views/:hop_id/align", h.align)
	   g.POST("/views/:hop_id/manual", h.markManual)
	   g.POST("/views/:hop_id/materialize", h.materialize)
  }

  func (h *Handler) listViews(c *gin.Context) {
	  q := MirrorQuery{
		  UserID: strings.TrimSpace(c.Query("user_id")),
		  Asset:  strings.TrimSpace(c.Query("asset")),
		  State:  MirrorState(strings.TrimSpace(c.Query("state"))),
		   Offset: parseIntQuery(c, "offset", 0),
		   Limit:  parseIntQuery(c, "limit", 50),
	   }
	  views, err := h.machine.store.List(c.Request.Context(), q)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"views": views, "count": len(views)})
   }

  func (h *Handler) getView(c *gin.Context) {
	  hopID := strings.TrimSpace(c.Param("hop_id"))
	  if hopID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "hop_id required"})
		  return
	   }
	   mv, err := h.machine.store.GetByHopID(c.Request.Context(), hopID)
	   if err != nil {
		   if IsNotFound(err) {
			   c.JSON(http.StatusNotFound, gin.H{"error": "mirror not found"})
			  return
		  }
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, mv)
  }

  func (h *Handler) getEvents(c *gin.Context) {
	  hopID := strings.TrimSpace(c.Param("hop_id"))
	  if hopID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "hop_id required"})
		   return
	   }
	   events, err := h.machine.GetEvents(c.Request.Context(), hopID)
	  if err != nil {
		  if IsNotFound(err) {
			   c.JSON(http.StatusNotFound, gin.H{"error": "mirror not found"})
			  return
		  }
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"events": events, "count": len(events)})
  }

  func (h *Handler) align(c *gin.Context) {
	  hopID := strings.TrimSpace(c.Param("hop_id"))
	   if hopID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "hop_id required"})
		   return
	  }
	   initiatorState := strings.TrimSpace(c.Query("initiator_state"))
	   if initiatorState == "" {
		   initiatorState = "processing"
	   }
	   result, err := h.machine.Align(c.Request.Context(), hopID, initiatorState)
	   if err != nil {
		   if IsNotFound(err) {
			  c.JSON(http.StatusNotFound, gin.H{"error": "mirror not found"})
			  return
		  }
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, result)
  }

// manualReq 人工推进请求体。
type manualReq struct {
	Actor  string `json:"actor" binding:"required"`
	Reason string `json:"reason"`
}

  func (h *Handler) markManual(c *gin.Context) {
	   hopID := strings.TrimSpace(c.Param("hop_id"))
	  if hopID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "hop_id required"})
		  return
	  }
	  var req manualReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	  mv,
		  err := h.machine.MarkManual(c.Request.Context(), hopID, req.Actor, req.Reason)
	  if err != nil {
		  if IsNotFound(err) {
			  c.JSON(http.StatusNotFound, gin.H{"error": "mirror not found"})
			  return
		  }
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, mv)
   }

   func (h *Handler) materialize(c *gin.Context) {
	  hopID := strings.TrimSpace(c.Param("hop_id"))
	  if hopID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "hop_id required"})
		   return
	   }
	   if err := h.pump.MaterializeOne(c.Request.Context(), hopID); err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"materialized": true})
   }

  func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	   s := strings.TrimSpace(c.Query(key))
	  if s == "" {
		  return defaultVal
	  }
	   v, err := strconv.Atoi(s)
	   if err != nil || v < 0 {
		   return defaultVal
	   }
	   return v
  }
