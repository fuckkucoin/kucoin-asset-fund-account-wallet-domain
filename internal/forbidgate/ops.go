package forbidgate

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
)

// OpsHandler 运维端点处理器。
type OpsHandler struct {
	gate  *Gate
	drift *DriftService
	metrics *Metrics
}

// NewOpsHandler 构造运维处理器。
  func NewOpsHandler(gate *Gate, drift *DriftService, metrics *Metrics) *OpsHandler {
	  return &OpsHandler{gate: gate, drift: drift, metrics: metrics}
  }

// RegisterOps 挂载运维路由。
   func (h *OpsHandler) RegisterOps(rg *gin.RouterGroup) {
	   ops := rg.Group("/ops/forbid-gate")
	  ops.GET("/health", h.health)
	  ops.GET("/metrics", h.metricsSnapshot)
	   ops.GET("/gen-batch", h.genBatch)
	   ops.GET("/drift-stats", h.driftStats)
	  ops.POST("/revoke-bind", h.revokeBind)
  }

// health GET /v1/ops/forbid-gate/health

  func (h *OpsHandler) health(c *gin.Context) {
	   c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "forbid-gate"})
  }

// metricsSnapshot GET /v1/ops/forbid-gate/metrics

   func (h *OpsHandler) metricsSnapshot(c *gin.Context) {
	  snap := h.metrics.Snapshot()
	   c.JSON(http.StatusOK, gin.H{"metrics": snap})
  }

// genBatch GET /v1/ops/forbid-gate/gen-batch?user_ids=u1,u2,u3
  func (h *OpsHandler) genBatch(c *gin.Context) {
	   raw := strings.TrimSpace(c.Query("user_ids"))
	   if raw == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids required"})
		  return
	  }
	   userIDs := splitAndTrim(raw, ",")
	  if len(userIDs) > 50 {
		  userIDs = userIDs[:50]
	   }
	  result := make(map[string]ForbidGenView, len(userIDs))
	  for _, uid := range userIDs {
		   view, err := h.gate.CurrentGeneration(c.Request.Context(), uid)
		  if err != nil {
			  result[uid] = ForbidGenView{UserID: uid}
			   continue
		   }
		  result[uid] = view
	   }
	  c.JSON(http.StatusOK, gin.H{"generations": result})
   }

// driftStats GET /v1/ops/forbid-gate/drift-stats?user_id=xxx&limit=20
  func (h *OpsHandler) driftStats(c *gin.Context) {
	   userID := strings.TrimSpace(c.Query("user_id"))
	  limitStr := c.DefaultQuery("limit", "20")
	  limit, _ := strconv.Atoi(limitStr)
	   if limit <= 0 || limit > 100 {
		  limit = 20
	   }
	   if h.drift == nil || h.drift.repo == nil {
		  c.JSON(http.StatusServiceUnavailable, gin.H{"error": "drift repo unavailable"})
		   return
	  }
	  var allOpen, proven, closed int
	  if userID != "" {
		  openCases,
			   _ := h.drift.repo.ListDriftCasesByUserState(c.Request.Context(), userID, DriftStateOpen)
		  provenCases, _ := h.drift.repo.ListDriftCasesByUserState(c.Request.Context(), userID, DriftStateProven)
		   closedCases, _ := h.drift.repo.ListDriftCasesByUserState(c.Request.Context(), userID, DriftStateClosed)
		  allOpen = len(openCases)
		   proven = len(provenCases)
		   closed = len(closedCases)
	   } else {
		  cases, _ := h.drift.repo.ListAllOpenDriftCases(c.Request.Context(), limit)
		  for _, dc := range cases {
			   switch dc.State {
			  case DriftStateOpen:
				  allOpen++
			  case DriftStateProven:
				   proven++
			   }
		   }
	  }
	  c.JSON(http.StatusOK, gin.H{
		   "open": allOpen,
		  "proven": proven,
		  "closed": closed,
	   })
  }

// revokeBindReq POST /v1/ops/forbid-gate/revoke-bind 请求体。
type revokeBindReq struct {
	BindID string `json:"bind_id"`
	Reason string `json:"reason"`
}

// revokeBind POST /v1/ops/forbid-gate/revoke-bind（运维强制吊销）。

   func (h *OpsHandler) revokeBind(c *gin.Context) {
	  var req revokeBindReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return
	   }
	   req.BindID = strings.TrimSpace(req.BindID)
	   if req.BindID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "bind_id required"})
		   return
	  }
	   if err := h.gate.RevokeBind(c.Request.Context(), req.BindID, req.Reason); err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"revoked": true})
  }

// splitAndTrim 按分隔符切分并去除空白。

   func splitAndTrim(s, sep string) []string {
	   parts := strings.Split(s, sep)
	  out := make([]string,
		  0, len(parts))
	   for _, p := range parts {
		   if t := strings.TrimSpace(p); t != "" {
			   out = append(out, t)
		  }
	   }
	   return out
  }
