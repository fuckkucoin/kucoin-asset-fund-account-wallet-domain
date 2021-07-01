package georebal

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// Handler 聚合 Planner 与 Executor 的 HTTP 视图。

type Handler struct {
	planner *Planner
	exec    *Executor
}

  func NewHandler(planner *Planner, exec *Executor) *Handler {
	   return &Handler{planner: planner, exec: exec}
  }

// Router 单独作为服务时使用。
   func (h *Handler) Router() *gin.Engine {
	   r := gin.New()
	  r.GET("/healthz", func(c *gin.Context) {
		  c.JSON(http.StatusOK, gin.H{"service": "geo-rebal", "ok": true})
	   })
	   h.mount(r.Group("/v1/geo-rebal"))
	  return r
  }

// Mount 挂在 svcboot RouterGroup 上。
  func (h *Handler) Mount(v1 *gin.RouterGroup) {
	   h.mount(v1.Group("/geo-rebal"))
  }

   func (h *Handler) mount(g *gin.RouterGroup) {
	  g.POST("/plans", h.createPlan)
	   g.GET("/plans", h.listPlans)
	  g.GET("/plans/:id", h.getPlan)
	  g.POST("/plans/:id/approve", h.approvePlan)
	   g.POST("/plans/:id/arm", h.armPlan)
	   g.POST("/plans/:id/abort", h.abortPlan)
	  g.POST("/plans/:id/run", h.runPlan)
	  g.POST("/plans/:id/legs/:seq/done", h.legDone)
  }

   func (h *Handler) createPlan(c *gin.Context) {
	   var req CreatePlanReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	  }
	   plan, err := h.planner.CreatePlan(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, plan)
   }

  func (h *Handler) listPlans(c *gin.Context) {
	   var states []string
	  if s := c.Query("state"); s != "" {
		  states = []string{s}
	   }
	   plans, err := h.planner.ListPlans(c.Request.Context(), states)
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"plans": plans})
  }

  func (h *Handler) getPlan(c *gin.Context) {
	   plan, err := h.planner.GetPlan(c.Request.Context(), c.Param("id"))
	   if err != nil {
		   writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, plan)
  }

   func (h *Handler) approvePlan(c *gin.Context) {
	  var req ApprovePlanReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	  req.PlanID = c.Param("id")
	  plan,
		  err := h.planner.ApprovePlan(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, plan)
   }

   func (h *Handler) armPlan(c *gin.Context) {
	   var req ArmPlanReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	   req.PlanID = c.Param("id")
	   plan, err := h.planner.ArmPlan(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, plan)
   }

   func (h *Handler) abortPlan(c *gin.Context) {
	  var req AbortPlanReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	   req.PlanID = c.Param("id")
	   plan, err := h.planner.AbortPlan(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, plan)
   }

   func (h *Handler) runPlan(c *gin.Context) {
	   if h.exec == nil {
		  c.JSON(http.StatusServiceUnavailable, gin.H{"error": "executor not configured"})
		   return
	   }
	  res, err := h.exec.RunPlan(c.Request.Context(), c.Param("id"))
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, res)
  }

  func (h *Handler) legDone(c *gin.Context) {
	  var req LegDoneReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	   }
	   req.PlanID = c.Param("id")
	   seq := 0
	   for _, r := range c.Param("seq") {
		   if r >= '0' && r <= '9' {
			   seq = seq*10 + int(r-'0')
		   }
	  }
	  req.LegSeq = seq
	  if h.exec == nil {
		   c.JSON(http.StatusServiceUnavailable, gin.H{"error": "executor not configured"})
		   return
	   }
	   res, err := h.exec.LegDone(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, res)
  }

  func writeErr(c *gin.Context, err error) {
	  code := CodeOf(err)
	  status := http.StatusInternalServerError
	   switch code {
	  case CodeNotFound:
		  status = http.StatusNotFound
	   case CodeInvalid, CodeConserve, CodeActor, CodeIdem:
		  status = http.StatusBadRequest
	   case CodeRegionHalted, CodeFirewallBlocked, CodeApproval, CodeBadTransition, CodeFirewall:
		  status = http.StatusForbidden
	   case CodeLegFailed:
		   status = http.StatusUnprocessableEntity
	  }
	   c.JSON(status, gin.H{"error": err.Error(), "code": code})
   }
