package cvrollback

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 处理器。
type Handler struct {
	orch *Orchestrator
}

// NewHandler 构造处理器。
   func NewHandler(orch *Orchestrator) *Handler {
	  return &Handler{orch: orch}
  }

// Register 注册路由（建议挂载于 /ops/cv-rollback）。
  func (h *Handler) Register(rg *gin.RouterGroup) {
	  rg.GET("/plan/:id", h.getPlan)
	  rg.GET("/plan/occ/:occ_id", h.getPlanByOcc)
	   rg.GET("/plans/pending", h.listPending)
	  rg.POST("/plan", h.createPlan)
	  rg.POST("/plan/:id/execute", h.executePlan)
   }

// getPlan 按 ID 查询回滚计划。

  func (h *Handler) getPlan(c *gin.Context) {
	  id := c.Param("id")
	  plan, err := h.orch.GetPlan(c.Request.Context(), id)
	   if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, plan)
  }

// getPlanByOcc 按 occID 查询回滚计划。
   func (h *Handler) getPlanByOcc(c *gin.Context) {
	  occID := c.Param("occ_id")
	  plan, err := h.orch.GetPlanByOcc(c.Request.Context(), occID)
	   if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	   }
	  c.JSON(http.StatusOK, plan)
   }

// listPending 列出待执行的回滚计划。
   func (h *Handler) listPending(c *gin.Context) {
	   limit := 50
	  if lq := c.Query("limit"); lq != "" {
		  if n, err := strconv.Atoi(lq); err == nil && n > 0 {
			   limit = n
		  }
	   }
	  plans, err := h.orch.ListPendingPlans(c.Request.Context(), limit)
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"plans": plans, "count": len(plans)})
   }

// createPlan 手动创建回滚计划（ops desk 强制回滚入口）。
// POST /ops/cv-rollback/plan
// Body: {"occ_id":"", "order_id":"", "user_id":"", "trigger":"MANUAL",
// "filled_amount":"", "remain_amount":"", "debit_idem_key":""}

   func (h *Handler) createPlan(c *gin.Context) {
	   var req PlanReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	  }
	  if req.Trigger == "" {
		   req.Trigger = TriggerManual
	  }
	   plan, err := h.orch.PlanAndExecute(c.Request.Context(), req)
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, plan)
   }

// executePlan 执行已有回滚计划。
// POST /ops/cv-rollback/plan/:id/execute
  func (h *Handler) executePlan(c *gin.Context) {
	   id := c.Param("id")
	   plan, err := h.orch.Execute(c.Request.Context(), id)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, plan)
  }
