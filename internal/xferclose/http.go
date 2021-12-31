// 功能：划转半截收口 HTTP/ops API。
// 路由（挂载到 v1 路由组下的 /xferclose 前缀）：
// GET /xferclose/hops 列出活跃半截工单
// GET /xferclose/hops/:hop_id 查询单个工单及其收口视图
// POST /xferclose/hops/:hop_id/closeout 强制收口（ops）
// GET /xferclose/plans 列出收口计划
// GET /xferclose/plans/:plan_id 查询单个计划及其腿
// GET /xferclose/slas 列出 SLA 规则
// POST /xferclose/slas 新增 SLA 规则
// PUT /xferclose/slas/:id 更新 SLA 规则
// DELETE /xferclose/slas/:id 删除 SLA 规则
// GET /xferclose/stats 统计看板
// 所有写操作需要鉴权头（通过 gin 中间件，本文件不重复实现）。

// 作者：Dax
// 日期：2021.12.30

package xferclose

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Handler HTTP 处理器集合。
type Handler struct {
	store Store
	orch *Orchestrator
	sla  *SLABoard
}

// NewHandler 构造 HTTP 处理器。

   func NewHandler(store Store, orch *Orchestrator, sla *SLABoard) *Handler {
	  return &Handler{store: store, orch: orch, sla: sla}
   }

// Register 将路由注册到 gin 路由组。

   func (h *Handler) Register(rg *gin.RouterGroup) {
	  g := rg.Group("/xferclose")
	  g.GET("/hops", h.listHops)
	  g.GET("/hops/:hop_id", h.getHop)
	  g.POST("/hops/:hop_id/closeout", h.forceCloseout)
	   g.GET("/plans", h.listPlans)
	  g.GET("/plans/:plan_id", h.getPlan)
	   g.GET("/slas", h.listSLAs)
	  g.POST("/slas", h.createSLA)
	  g.PUT("/slas/:id", h.updateSLA)
	  g.DELETE("/slas/:id", h.deleteSLA)
	   g.GET("/stats", h.stats)
   }

// ── HalfHop ──────────────────────────────────────────────────────────────────

  func (h *Handler) listHops(c *gin.Context) {
	  offset := parseIntQuery(c, "offset", 0)
	   limit := parseIntQuery(c, "limit", 50)
	  state := HopState(strings.TrimSpace(c.Query("state")))
	   var (
		  hops []HalfHop
		  err  error
	   )
	   if state != "" {
		  hops, err = h.store.ListByState(c.Request.Context(), state, offset, limit)
	   } else {
		  hops, err = h.store.ListActive(c.Request.Context(), offset, limit)
	   }
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"hops": hops, "count": len(hops)})
   }

  func (h *Handler) getHop(c *gin.Context) {
	   hopID := strings.TrimSpace(c.Param("hop_id"))
	  if hopID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "hop_id required"})
		  return
	  }
	   view, err := h.orch.GetHopCloseoutView(c.Request.Context(), hopID)
	  if err != nil {
		   if IsNotFound(err) {
			   c.JSON(http.StatusNotFound, gin.H{"error": "hop not found"})
			  return
		   }
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	   c.JSON(http.StatusOK, view)
  }

// forceCloseoutReq 强制收口请求体。
type forceCloseoutReq struct {
	Actor  string         `json:"actor" binding:"required"`
	Action CloseoutAction `json:"action"`
}

   func (h *Handler) forceCloseout(c *gin.Context) {
	   hopID := strings.TrimSpace(c.Param("hop_id"))
	   if hopID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "hop_id required"})
		  return
	  }
	  var req forceCloseoutReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	  result, err := h.orch.ForceCloseout(c.Request.Context(), hopID, req.Actor, req.Action)
	  if err != nil {
		   status := http.StatusInternalServerError
		  if IsNotFound(err) {
			   status = http.StatusNotFound
		  } else if err == ErrAlreadyTerminal || err == ErrPlanDone {
			  status = http.StatusConflict
		  }
		  c.JSON(status, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, result)
  }

// ── CloseoutPlan ─────────────────────────────────────────────────────────────

   func (h *Handler) listPlans(c *gin.Context) {
	   stateStr := strings.TrimSpace(c.Query("state"))
	  if stateStr == "" {
		  stateStr = string(PlanRunning)
	  }
	  state := PlanState(stateStr)
	  offset := parseIntQuery(c, "offset", 0)
	  limit := parseIntQuery(c, "limit", 50)
	   plans, err := h.store.ListPlans(c.Request.Context(), state, offset, limit)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"plans": plans, "count": len(plans)})
   }

  func (h *Handler) getPlan(c *gin.Context) {
	   planID := strings.TrimSpace(c.Param("plan_id"))
	   if planID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id required"})
		  return
	  }
	  plan, err := h.store.GetPlan(c.Request.Context(), planID)
	  if err != nil {
		   if IsNotFound(err) {
			  c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
			   return
		   }
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	   legs, _ := h.store.ListLegs(c.Request.Context(), planID)
	  c.JSON(http.StatusOK, gin.H{"plan": plan, "legs": legs})
   }

// ── SLA ───────────────────────────────────────────────────────────────────────

   func (h *Handler) listSLAs(c *gin.Context) {
	   slas, err := h.store.ListSLAs(c.Request.Context())
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"slas": slas, "count": len(slas)})
  }

// createSLAReq SLA 创建请求。
type createSLAReq struct {
	Name string `json:"name" binding:"required"`
	Asset string `json:"asset"`
	FromBook string `json:"from_book"`
	ToBook   string `json:"to_book"`
	AmtMin string `json:"amt_min"`
	AmtMax        string `json:"amt_max"`
	TimeoutSec    int64  `json:"timeout_sec" binding:"required"`
	EscalateSec   int64  `json:"escalate_sec"`
	AutoCloseout  bool   `json:"auto_closeout"`
	DefaultAction string `json:"default_action" binding:"required"`
	Priority      int    `json:"priority"`
}

   func (h *Handler) createSLA(c *gin.Context) {
	  var req createSLAReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		   return
	  }
	   now := time.Now().UTC()
	  amtMin, _ := decimal.NewFromString(req.AmtMin)
	   amtMax, _ := decimal.NewFromString(req.AmtMax)
	  r := XferSLA{
		   ID: uuid.NewString(),
		   Name: req.Name,
		   Asset: strings.ToUpper(req.Asset),
		   FromBook: strings.ToLower(req.FromBook),
		  ToBook:   strings.ToLower(req.ToBook),
		  AmtMin:        amtMin,
		   AmtMax:        amtMax,
		  Timeout:       time.Duration(req.TimeoutSec) * time.Second,
		   EscalateAfter: time.Duration(req.EscalateSec) * time.Second,
		  AutoCloseout:  req.AutoCloseout,
		  DefaultAction: CloseoutAction(req.DefaultAction),
		   Enabled:       true,
		   Priority:      req.Priority,
		  CreatedAt:     now,
		  UpdatedAt:     now,
	   }
	  if err := h.store.PutSLA(c.Request.Context(), r); err != nil {
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		   return
	   }
	  h.sla.Upsert(r)
	  c.JSON(http.StatusCreated, r)
   }

  func (h *Handler) updateSLA(c *gin.Context) {
	  id := strings.TrimSpace(c.Param("id"))
	  if id == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		   return
	  }
	  existing, err := h.store.GetSLA(c.Request.Context(), id)
	   if err != nil {
		  if IsNotFound(err) {
			  c.JSON(http.StatusNotFound, gin.H{"error": "sla not found"})
			  return
		  }
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	  var req createSLAReq
	   if err2 := c.ShouldBindJSON(&req); err2 != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err2.Error()})
		  return
	  }
	  amtMin, _ := decimal.NewFromString(req.AmtMin)
	   amtMax, _ := decimal.NewFromString(req.AmtMax)
	   existing.Name = req.Name
	   existing.Asset = strings.ToUpper(req.Asset)
	   existing.FromBook = strings.ToLower(req.FromBook)
	  existing.ToBook = strings.ToLower(req.ToBook)
	  existing.AmtMin = amtMin
	  existing.AmtMax = amtMax
	   existing.Timeout = time.Duration(req.TimeoutSec) * time.Second
	  existing.EscalateAfter = time.Duration(req.EscalateSec) * time.Second
	  existing.AutoCloseout = req.AutoCloseout
	  existing.DefaultAction = CloseoutAction(req.DefaultAction)
	   existing.Priority = req.Priority
	   existing.UpdatedAt = time.Now().UTC()
	   if err2 := h.store.UpdateSLA(c.Request.Context(), existing); err2 != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err2.Error()})
		   return
	   }
	   h.sla.Upsert(existing)
	  c.JSON(http.StatusOK, existing)
   }

   func (h *Handler) deleteSLA(c *gin.Context) {
	  id := strings.TrimSpace(c.Param("id"))
	   if id == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		  return
	  }
	  if err := h.store.DeleteSLA(c.Request.Context(), id); err != nil {
		   if IsNotFound(err) {
			   c.JSON(http.StatusNotFound, gin.H{"error": "sla not found"})
			  return
		  }
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	  }
	  h.sla.Delete(id)
	  c.JSON(http.StatusOK, gin.H{"deleted": id})
   }

// ── Stats ──────────────────────────────────────────────────────────────────
// StatsView 统计看板视图。
type StatsView struct {
	ActiveHops   int `json:"active_hops"`
	BreachedHops int `json:"breached_hops"`
	ClosedHops   int `json:"closed_hops"`
	EscalatedHops int `json:"escalated_hops"`
	OpenPlans int `json:"open_plans"`
	RunningPlans int `json:"running_plans"`
	DonePlans int `json:"done_plans"`
	AbortedPlans int `json:"aborted_plans"`
}

  func (h *Handler) stats(c *gin.Context) {
	  ctx := c.Request.Context()
	   active, _ := h.store.ListActive(ctx, 0, 1000)
	  closed, _ := h.store.ListByState(ctx, HopClosed, 0, 1000)
	   escalated,
		   _ := h.store.ListByState(ctx, HopEscalated, 0, 1000)
	   openPlans, _ := h.store.ListPlans(ctx, PlanOpen, 0, 1000)
	   runningPlans, _ := h.store.ListPlans(ctx, PlanRunning, 0, 1000)
	   donePlans, _ := h.store.ListPlans(ctx, PlanDone, 0, 1000)
	   abortedPlans, _ := h.store.ListPlans(ctx, PlanAborted, 0, 1000)
	   breached := 0
	  for _, h2 := range active {
		  if h2.SLABreached {
			  breached++
		  }
	  }
	  view := StatsView{
		   ActiveHops: len(active),
		  BreachedHops:  breached,
		   ClosedHops:    len(closed),
		   EscalatedHops: len(escalated),
		  OpenPlans:     len(openPlans),
		   RunningPlans: len(runningPlans),
		   DonePlans:    len(donePlans),
		   AbortedPlans: len(abortedPlans),
	  }
	  c.JSON(http.StatusOK, view)
  }

// ── helpers ───────────────────────────────────────────────────────────────────

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
// temporary guard removed after the production check
