package reconflow

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 在 v1 RouterGroup 下挂载对账差异处置工作流 API。
// 调用方传入已构造好的 Engine。
   func Register(v1 *gin.RouterGroup, engine *Engine) {
	   h := &handler{engine: engine}
	   // 分类接口
	  v1.POST("/reconflow/classify", h.classify)
	  // FlowDef CRUD
	   v1.GET("/reconflow/defs", h.listFlowDefs)
	  v1.POST("/reconflow/defs", h.upsertFlowDef)
	  v1.GET("/reconflow/defs/:id", h.getFlowDef)
	   v1.DELETE("/reconflow/defs/:id", h.deleteFlowDef)
	  // ActionDef 目录
	   v1.GET("/reconflow/actions", h.listActionDefs)
	   v1.POST("/reconflow/actions", h.upsertActionDef)
	   // FlowRun 查询
	  v1.GET("/reconflow/runs", h.listRuns)
	  v1.GET("/reconflow/runs/:id", h.getRun)
	   v1.GET("/reconflow/runs/:id/steps", h.listSteps)
	  // 人工放行（gate ops）
	  v1.GET("/reconflow/gates", h.listGatedSteps)
	  v1.POST("/reconflow/gates/allow", h.gateAllow)
	   v1.POST("/reconflow/gates/deny", h.gateDeny)
	  // SLA 状态面
	   v1.GET("/reconflow/sla", h.slaView)
  }

type handler struct {
	engine *Engine
}

// ──────────────────────────────────────────────
// classify
// ──────────────────────────────────────────────

   func (h *handler) classify(c *gin.Context) {
	  actor, err := requireOpsView(c)
	   if err != nil {
		  return
	  }
	   var req ClassifyRequest
	  if !mw.BindJSON(c, &req) {
		  return
	  }
	  if req.Feature.SourceKind == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "feature.source_kind required"})
		  return
	   }
	  result,
		  err := h.engine.Classify(c.Request.Context(), req, actor)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"result": result})
   }

// ──────────────────────────────────────────────
// FlowDef CRUD
// ──────────────────────────────────────────────

   func (h *handler) listFlowDefs(c *gin.Context) {
	   if _, err := requireOpsView(c); err != nil {
		   return
	  }
	   defs, err := h.engine.ListFlowDefs(c.Request.Context())
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"defs": defs})
   }

   func (h *handler) getFlowDef(c *gin.Context) {
	   if _, err := requireOpsView(c); err != nil {
		  return
	   }
	  id := c.Param("id")
	   d, err := h.engine.GetFlowDef(c.Request.Context(), id)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"def": d})
   }

   func (h *handler) upsertFlowDef(c *gin.Context) {
	  actor, err := requireOpsAdmin(c)
	   if err != nil {
		   return
	   }
	  var body struct {
		  Def FlowDef `json:"def"`
	  }
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	   if body.Def.Name == "" || body.Def.DiffKind == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "def.name and def.diff_kind required"})
		  return
	  }
	   if body.Def.SLA <= 0 {
		   body.Def.SLA = 4 * time.Hour
	   }
	   if err := h.engine.UpsertFlowDef(c.Request.Context(), body.Def, actor); err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"ok": true})
  }

   func (h *handler) deleteFlowDef(c *gin.Context) {
	   actor, err := requireOpsAdmin(c)
	  if err != nil {
		  return
	   }
	  id := c.Param("id")
	   if err := h.engine.DeleteFlowDef(c.Request.Context(), id, actor); err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"ok": true})
  }

// ──────────────────────────────────────────────
// ActionDef 目录
// ──────────────────────────────────────────────

   func (h *handler) listActionDefs(c *gin.Context) {
	  if _, err := requireOpsView(c); err != nil {
		   return
	  }
	  defs := h.engine.catalog.All()
	  c.JSON(http.StatusOK, gin.H{"actions": defs})
   }

   func (h *handler) upsertActionDef(c *gin.Context) {
	   _, err := requireOpsAdmin(c)
	  if err != nil {
		   return
	   }
	   var body struct {
		  Action ActionDef `json:"action"`
	  }
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  if body.Action.Name == "" || body.Action.Kind == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "action.name and action.kind required"})
		   return
	  }
	   if err := h.engine.UpsertActionDef(c.Request.Context(), body.Action); err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"ok": true})
  }

// ──────────────────────────────────────────────
// FlowRun 查询
// ──────────────────────────────────────────────

   func (h *handler) listRuns(c *gin.Context) {
	  if _, err := requireOpsView(c); err != nil {
		  return
	  }
	   q := RunQuery{
		   UserID: c.Query("user_id"),
		  Asset:    c.Query("asset"),
		   DiffKind: c.Query("diff_kind"),
		   TicketID: c.Query("ticket_id"),
	   }
	   if s := c.Query("state"); s != "" {
		   q.State = RunState(s)
	  }
	  q.Limit = queryInt(c, "limit", 50)
	   q.Offset = queryInt(c, "offset", 0)
	  if q.Limit > 200 {
		  q.Limit = 200
	   }
	  runs,
		   err := h.engine.ListRuns(c.Request.Context(), q)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"runs": runs, "limit": q.Limit, "offset": q.Offset})
  }

   func (h *handler) getRun(c *gin.Context) {
	   if _, err := requireOpsView(c); err != nil {
		   return
	  }
	   id := c.Param("id")
	   run,
		  err := h.engine.GetRun(c.Request.Context(), id)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"run": run})
   }

   func (h *handler) listSteps(c *gin.Context) {
	  if _, err := requireOpsView(c); err != nil {
		  return
	   }
	  runID := c.Param("id")
	   steps, err := h.engine.ListSteps(c.Request.Context(), runID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"steps": steps})
  }

// ──────────────────────────────────────────────
// 人工放行（gate ops）
// ──────────────────────────────────────────────

   func (h *handler) listGatedSteps(c *gin.Context) {
	   if _, err := requireOpsView(c); err != nil {
		   return
	   }
	  limit := queryInt(c, "limit", 50)
	  steps, err := h.engine.ListGatedSteps(c.Request.Context(), limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"gated_steps": steps, "count": len(steps)})
   }

  func (h *handler) gateAllow(c *gin.Context) {
	  actor, err := requireOpsAdmin(c)
	   if err != nil {
		   return
	  }
	  var dec GateDecision
	   if !mw.BindJSON(c, &dec) {
		   return
	  }
	  if dec.RunID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "run_id required"})
		   return
	  }
	   dec.Actor = actor
	  dec.Allow = true
	   if err := h.engine.GateAllow(c.Request.Context(), dec); err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"ok": true, "run_id": dec.RunID})
  }

   func (h *handler) gateDeny(c *gin.Context) {
	   actor, err := requireOpsAdmin(c)
	  if err != nil {
		  return
	  }
	  var dec GateDecision
	   if !mw.BindJSON(c, &dec) {
		  return
	  }
	   if dec.RunID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "run_id required"})
		  return
	  }
	  dec.Actor = actor
	  dec.Allow = false
	   if err := h.engine.GateDeny(c.Request.Context(), dec); err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"ok": true, "run_id": dec.RunID})
  }

// ──────────────────────────────────────────────
// SLA 状态面
// ──────────────────────────────────────────────

   func (h *handler) slaView(c *gin.Context) {
	   if _, err := requireOpsView(c); err != nil {
		   return
	  }
	  ctx := c.Request.Context()
	  var (
		  active   []FlowRun
		  breached []FlowRun
	  )
	  now := time.Now().UTC()
	  if h.engine.store != nil {
		   active,
			  _ = h.engine.store.ListActiveRuns(ctx, 200)
		  breached, _ = h.engine.store.ListBreachedRuns(ctx, now, 100)
	   }
	   summary := map[string]int{
		  "active":   len(active),
		   "breached": len(breached),
	  }
	  c.JSON(http.StatusOK, gin.H{
		  "summary": summary,
		  "now":     now,
		  "host":    "ledger-transfer-freeze-reconciliation.kucoin.net",
	  })
   }

// ──────────────────────────────────────────────
// 权限工具
// ──────────────────────────────────────────────

  func requireOpsAdmin(c *gin.Context) (string, error) {
	   actor := perm.FromRequest(c.Request, "")
	   if err := actor.Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return "", err
	   }
	   if strings.TrimSpace(actor.ID) == "" {
		   c.JSON(http.StatusForbidden, gin.H{"error": "actor required"})
		   return "", ErrActor
	  }
	   return actor.ID, nil
   }

   func requireOpsView(c *gin.Context) (string, error) {
	  actor := perm.FromRequest(c.Request, "")
	   // OpsAdmin 也有 view 权限；先检查 Admin，回退检查 QueryRead。
	   if actor.Check(perm.ActOpsAdmin) == nil {
		  return actor.ID, nil
	   }
	  if err := actor.Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return "", err
	   }
	  return actor.ID, nil
   }

  func queryInt(c *gin.Context, key string, def int) int {
	  s := c.Query(key)
	   if s == "" {
		   return def
	  }
	  v, err := strconv.Atoi(s)
	  if err != nil || v < 0 {
		   return def
	   }
	  return v
  }
