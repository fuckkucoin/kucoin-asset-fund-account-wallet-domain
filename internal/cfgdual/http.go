package cfgdual

import (
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cfgcenter"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Register 注册灰度双评路由，并向 cfgcenter.Store 订阅事件钩子。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  cfgSt := cfgcenter.NewStore()
	  if rt != nil && !rt.Mem && rt.SQL != nil {
		   cfgSt.BindSQL(rt.SQL)
	  }
	  idgen := func() string { return uuid.NewString() }
	  var mgr *Manager
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		   mgr = NewManager(cfgSt, rt.SQL, idgen)
	  } else {
		   mgr = NewManager(cfgSt, nil, idgen)
	  }
	  h := &handler{mgr: mgr}
	  g := v1.Group("/cfgdual")
	   // ── 发布管理 ──────────────────────────────────────────────────────────
	  g.GET("/releases", h.listReleases)
	  g.POST("/releases", h.createRelease)
	   g.GET("/releases/:id", h.getRelease)
	   g.POST("/releases/:id/activate", h.activateRelease)
	   // ── Cohort 切流 ──────────────────────────────────────────────────────
	  g.GET("/releases/:id/cohort", h.getCohort)
	   g.POST("/releases/:id/cohort", h.updateCohort)
	  // ── 差分看板 ─────────────────────────────────────────────────────────
	   g.GET("/releases/:id/diffs", h.listDiffs)
	   // ── 封堵 ─────────────────────────────────────────────────────────────
	   g.GET("/releases/:id/seals", h.listSealActions)
	  g.POST("/releases/:id/seals/waive", h.waiveSeal)
	   // ── 回滚 ─────────────────────────────────────────────────────────────
	   g.POST("/releases/:id/rollback", h.rollback)
	  g.GET("/releases/:id/proof", h.getProof)
	  // ── 双评与强制 baseline ───────────────────────────────────────────────
	  g.POST("/eval", h.eval)
	  g.POST("/baseline", h.baselineOnly)
  }

// ─────────────────────────────────────────────────────────────────────────────
// handler
// ─────────────────────────────────────────────────────────────────────────────

type handler struct {
	mgr *Manager
}

   func (h *handler) requireOps(c *gin.Context) bool {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return false
	   }
	   return true
   }

   func (h *handler) requireRead(c *gin.Context) bool {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return false
	  }
	   return true
   }

// GET /cfgdual/releases
  func (h *handler) listReleases(c *gin.Context) {
	  if !h.requireOps(c) {
		  return
	   }
	   list, err := h.mgr.Store.ListReleases(c.Request.Context())
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"releases": list})
   }

// POST /cfgdual/releases
  func (h *handler) createRelease(c *gin.Context) {
	   if !h.requireOps(c) {
		  return
	   }
	   var req CreateReleaseRequest
	   if !mw.BindJSON(c, &req) {
		  return
	  }
	  r, err := h.mgr.CreateRelease(c.Request.Context(), req)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, r)
  }

// GET /cfgdual/releases/:id

  func (h *handler) getRelease(c *gin.Context) {
	  if !h.requireRead(c) {
		  return
	  }
	  r, err := h.mgr.Store.GetRelease(c.Request.Context(), c.Param("id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, r)
  }

// POST /cfgdual/releases/:id/activate
  func (h *handler) activateRelease(c *gin.Context) {
	   if !h.requireOps(c) {
		   return
	   }
	  r, err := h.mgr.Store.GetRelease(c.Request.Context(), c.Param("id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   if err := h.mgr.ActivateRelease(c.Request.Context(), r); err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, r)
  }

// GET /cfgdual/releases/:id/cohort
   func (h *handler) getCohort(c *gin.Context) {
	  if !h.requireRead(c) {
		  return
	  }
	  cohort, err := h.mgr.Cohorts.Get(c.Param("id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, cohort.Snapshot())
   }

// POST /cfgdual/releases/:id/cohort
   func (h *handler) updateCohort(c *gin.Context) {
	  if !h.requireOps(c) {
		  return
	  }
	  var body struct {
		   Percent *int `json:"percent"`
		   PinUser string `json:"pin_user"`
		   PinAsset string `json:"pin_asset"`
		  PinSide  string `json:"pin_side"`
		  UnpinUser string `json:"unpin_user"`
		   UnpinAsset string            `json:"unpin_asset"`
		  UserPins   map[string]string `json:"user_pins"`
	  }
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	   cohort, err := h.mgr.Cohorts.Get(c.Param("id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  if body.Percent != nil {
		   if _, err := cohort.SetPercent(*body.Percent); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
	   }
	   if body.PinUser != "" {
		  if err := cohort.PinUser(body.PinUser, body.PinSide); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
	   }
	  if body.PinAsset != "" {
		   if err := cohort.PinAsset(body.PinAsset, body.PinSide); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
	   }
	  if body.UnpinUser != "" {
		   cohort.UnpinUser(body.UnpinUser)
	  }
	   if body.UnpinAsset != "" {
		   cohort.UnpinAsset(body.UnpinAsset)
	  }
	   for uid, side := range body.UserPins {
		  _ = cohort.PinUser(uid, side)
	  }
	   c.JSON(http.StatusOK, cohort.Snapshot())
  }

// GET /cfgdual/releases/:id/diffs

  func (h *handler) listDiffs(c *gin.Context) {
	  if !h.requireRead(c) {
		  return
	   }
	  limit := 50
	  if s := c.Query("limit"); s != "" {
		  if n, err := strconv.Atoi(s); err == nil && n > 0 {
			  limit = n
		  }
	  }
	  diffs, err := h.mgr.Store.ListSplitDiffs(c.Request.Context(), c.Param("id"), limit)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"diffs": diffs})
  }

// GET /cfgdual/releases/:id/seals
   func (h *handler) listSealActions(c *gin.Context) {
	   if !h.requireRead(c) {
		   return
	   }
	   actions, err := h.mgr.Store.ListSealActions(c.Request.Context(), c.Param("id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  gate := h.mgr.Seals.Get(c.Param("id"))
	   state := SealOpen
	   if gate != nil {
		   state = gate.State()
	  }
	  c.JSON(http.StatusOK, gin.H{"state": state, "actions": actions})
   }

// POST /cfgdual/releases/:id/seals/waive
   func (h *handler) waiveSeal(c *gin.Context) {
	  if !h.requireOps(c) {
		   return
	  }
	   var body struct {
		   Reason string `json:"reason"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  actor := mw.ActorFrom(c).ID
	   gate := h.mgr.Seals.GetOrCreate(c.Param("id"))
	  act, err := gate.Waive(body.Reason, actor)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  if act != nil {
		   if err := h.mgr.Store.SaveSealAction(c.Request.Context(), act); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
	   }
	  c.JSON(http.StatusOK, gin.H{"state": gate.State()})
   }

// POST /cfgdual/releases/:id/rollback

   func (h *handler) rollback(c *gin.Context) {
	  if !h.requireOps(c) {
		  return
	  }
	   var body struct {
		   Reason string `json:"reason"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	   rel, err := h.mgr.Store.GetRelease(c.Request.Context(), c.Param("id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  actor := mw.ActorFrom(c).ID
	   if actor == "" {
		  actor = "ops"
	   }
	  proof, err := h.mgr.Rollback.Rollback(c.Request.Context(), rel, "", actor, body.Reason)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, proof)
  }

// GET /cfgdual/releases/:id/proof

  func (h *handler) getProof(c *gin.Context) {
	  if !h.requireRead(c) {
		  return
	   }
	   proof, err := h.mgr.Rollback.GetProof(c.Request.Context(), c.Param("id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, proof)
   }

// POST /cfgdual/eval

  func (h *handler) eval(c *gin.Context) {
	   if !h.requireRead(c) {
		   return
	   }
	  var body struct {
		   ReleaseID string `json:"release_id"`
		  Req EvalRequest `json:"request"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  result, err := h.mgr.Engine.Eval(c.Request.Context(), body.ReleaseID, body.Req)
	   if err != nil && result.Sealed {
		  // Seal 触发：返回 409 Conflict + 结果体。
		  c.JSON(http.StatusConflict,
			   result)
		   return
	  }
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, result)
   }

// POST /cfgdual/baseline
   func (h *handler) baselineOnly(c *gin.Context) {
	   if !h.requireRead(c) {
		  return
	  }
	  var body EvalRequest
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  // 传空 releaseID → evalBaselineOnly 降级路径。
	   result, err := h.mgr.Engine.Eval(c.Request.Context(), "", body)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, result)
  }
