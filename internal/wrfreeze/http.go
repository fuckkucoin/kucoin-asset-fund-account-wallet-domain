package wrfreeze

import (
	"context"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 注册级联冻结协议路由。

   func RegisterRoutes(v1 *gin.RouterGroup, rt *app.Runtime) {
	   var (
		   store      Store = NewMemStore()
		  freezer    LedgerFreezer
		   holdReader HoldcatReader = NoopHoldcatReader{}
		  dual *dualctl.Book
	  )
	  if rt != nil {
		   if rt.Ledger != nil {
			   freezer = NewLedgerFreezerHook(rt.Ledger)
		  }
		   dual = dualctl.New(4 * time.Hour)
	   }
	   if freezer == nil {
		   freezer = &noopFreezer{}
	   }
	  planSvc := NewPlanService(store, dual, freezer)
	  waveSvc := NewWaveService(store, freezer, 8)
	   unfreezeSvc := NewUnfreezeService(store, freezer)
	  proofSvc := NewProofService(store, freezer)
	  rollbackSvc := NewRollbackService(store, freezer, holdReader)
	  compatSvc := NewCompatService(nil, store) // legacy reader 由调用方注入
	  h := &handler{
		   plan: planSvc, wave: waveSvc, unfreeze: unfreezeSvc,
		  proof: proofSvc, rollback: rollbackSvc, compat: compatSvc,
	   }
	  g := v1.Group("/wrfreeze")
	  {
		  // 计划管理
		   g.POST("/plans", h.authFreeze, h.openPlan)
		   g.POST("/plans/:id/arm", h.authFreeze, h.armPlan)
		   g.POST("/plans/:id/execute", h.authFreeze, h.executePlan)
		   g.POST("/plans/:id/abort", h.authFreeze, h.abortPlan)
		  g.POST("/plans/:id/seal", h.authFreeze, h.sealPlan)
		   g.GET("/plans/:id", h.authRead, h.getPlan)
		  g.GET("/plans", h.authRead, h.listPlans)
		   // 波次管理
		  g.POST("/plans/:id/waves", h.authFreeze, h.addWave)
		  g.GET("/plans/:id/waves", h.authRead, h.listWaves)
		  g.POST("/waves/:wid/run", h.authFreeze, h.runWave)
		   // 解冻阶段
		  g.POST("/plans/:id/phases", h.authFreeze, h.initPhase)
		   g.POST("/phases/:pid/approve", h.authFreeze, h.approvePhase)
		   g.GET("/plans/:id/phases", h.authRead, h.listPhases)
		  // 覆盖率证明
		   g.POST("/plans/:id/proofs", h.authFreeze, h.computeProof)
		   g.GET("/plans/:id/proofs", h.authRead, h.listProofs)
		   g.GET("/plans/:id/proofs/latest", h.authRead, h.latestProof)
		  // 回滚腿
		   g.POST("/plans/:id/rollbacks", h.authFreeze, h.requestRollback)
		  g.POST("/rollbacks/:lid/execute", h.authFreeze, h.executeRollback)
		   g.GET("/plans/:id/rollbacks", h.authRead, h.listLegs)
		  // FreezeAll 兼容投影
		  g.GET("/global-projection", h.authRead, h.globalProjection)
	   }
   }

type handler struct {
	plan *PlanService
	wave *WaveService
	unfreeze *UnfreezeService
	proof    *ProofService
	rollback *RollbackService
	compat   *CompatService
}

  func (h *handler) authFreeze(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		  c.Abort()
	   }
   }

   func (h *handler) authRead(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  c.Abort()
	   }
  }

// ── Plan Handlers ──

   func (h *handler) openPlan(c *gin.Context) {
	   var req OpenPlanReq
	   if !mw.BindJSON(c, &req) {
		   return
	  }
	  plan, dualID, err := h.plan.OpenPlan(c.Request.Context(), req)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(201, gin.H{"plan": plan, "dual_req_id": dualID})
   }

  func (h *handler) armPlan(c *gin.Context) {
	   req := ArmPlanReq{PlanID: c.Param("id")}
	   if !mw.BindJSON(c, &req) {
		  return
	  }
	  req.PlanID = c.Param("id")
	  plan, err := h.plan.ArmPlan(c.Request.Context(), req)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(200, plan)
  }

   func (h *handler) executePlan(c *gin.Context) {
	   req := ExecutePlanReq{PlanID: c.Param("id")}
	  if !mw.BindJSON(c, &req) {
		  return
	   }
	   req.PlanID = c.Param("id")
	   plan, err := h.plan.ExecutePlan(c.Request.Context(), req)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, plan)
  }

  func (h *handler) abortPlan(c *gin.Context) {
	   req := AbortPlanReq{PlanID: c.Param("id")}
	  if !mw.BindJSON(c, &req) {
		   return
	   }
	   req.PlanID = c.Param("id")
	   plan, err := h.plan.AbortPlan(c.Request.Context(), req)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(200, plan)
  }

   func (h *handler) sealPlan(c *gin.Context) {
	   var body struct {
		  Actor string `json:"actor"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  plan, err := h.plan.SealPlan(c.Request.Context(), c.Param("id"), body.Actor)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(200, plan)
  }

   func (h *handler) getPlan(c *gin.Context) {
	  plan, err := h.plan.GetPlan(c.Request.Context(), c.Param("id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(200, plan)
  }

   func (h *handler) listPlans(c *gin.Context) {
	   plans, err := h.plan.ListPlans(c.Request.Context(), c.Query("status"), 50)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(200, gin.H{"plans": plans})
  }

// ── Wave Handlers ──

  func (h *handler) addWave(c *gin.Context) {
	   var body struct {
		  Label       string   `json:"label"`
		  TargetUsers []string `json:"target_users"`
		   Actor       string   `json:"actor"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	  wave, err := h.wave.AddWave(c.Request.Context(), c.Param("id"), body.Label, body.TargetUsers, body.Actor)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(201, wave)
   }

  func (h *handler) listWaves(c *gin.Context) {
	   waves, err := h.wave.ListWaves(c.Request.Context(), c.Param("id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, gin.H{"waves": waves})
   }

  func (h *handler) runWave(c *gin.Context) {
	   var body struct {
		   Actor string `json:"actor"`
	   }
	  _ = mw.BindJSON(c, &body)
	  wave, err := h.wave.RunWave(c.Request.Context(), c.Param("wid"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(200, wave)
   }

// ── Phase Handlers ──

  func (h *handler) initPhase(c *gin.Context) {
	   var req InitPhaseReq
	   if !mw.BindJSON(c, &req) {
		  return
	  }
	  req.PlanID = c.Param("id")
	   phase, err := h.unfreeze.InitPhase(c.Request.Context(), req)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(201, phase)
  }

  func (h *handler) approvePhase(c *gin.Context) {
	  req := ApprovePhaseReq{PhaseID: c.Param("pid")}
	  if !mw.BindJSON(c, &req) {
		  return
	   }
	   req.PhaseID = c.Param("pid")
	  phase, err := h.unfreeze.ApprovePhase(c.Request.Context(), req)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(200, phase)
   }

  func (h *handler) listPhases(c *gin.Context) {
	  phases, err := h.unfreeze.ListPhases(c.Request.Context(), c.Param("id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(200, gin.H{"phases": phases})
  }

// ── Proof Handlers ──

   func (h *handler) computeProof(c *gin.Context) {
	   proof, err := h.proof.ComputeProof(c.Request.Context(), c.Param("id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(201, proof.ToSummary())
   }

  func (h *handler) listProofs(c *gin.Context) {
	   proofs, err := h.proof.ListProofs(c.Request.Context(), c.Param("id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  summaries := make([]ProofSummary, len(proofs))
	  for i, p := range proofs {
		   summaries[i] = p.ToSummary()
	  }
	   c.JSON(200, gin.H{"proofs": summaries})
   }

  func (h *handler) latestProof(c *gin.Context) {
	  proof, err := h.proof.LatestProof(c.Request.Context(), c.Param("id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   if proof == nil {
		   c.JSON(200, gin.H{"proof": nil})
		   return
	   }
	   c.JSON(200, gin.H{"proof": proof.ToSummary()})
  }

// ── Rollback Handlers ──

  func (h *handler) requestRollback(c *gin.Context) {
	  var req RollbackReq
	  if !mw.BindJSON(c, &req) {
		   return
	   }
	   req.PlanID = c.Param("id")
	   leg, err := h.rollback.RequestRollback(c.Request.Context(), req)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(201, leg)
   }

  func (h *handler) executeRollback(c *gin.Context) {
	  req := ExecuteRollbackReq{LegID: c.Param("lid")}
	  if !mw.BindJSON(c, &req) {
		  return
	  }
	   req.LegID = c.Param("lid")
	   leg, err := h.rollback.ExecuteRollback(c.Request.Context(), req)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(200, leg)
  }

  func (h *handler) listLegs(c *gin.Context) {
	  legs, err := h.rollback.ListLegs(c.Request.Context(), c.Param("id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(200, gin.H{"rollback_legs": legs})
  }

// ── Compat Handlers ──

   func (h *handler) globalProjection(c *gin.Context) {
	  proj, err := h.compat.GlobalProjection(c.Request.Context())
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(200, proj)
  }

// ── noopFreezer (fallback when ledger unavailable) ──

type noopFreezer struct{}

  func (n *noopFreezer) SetFrozen(context.Context, string, bool) error  { return nil }
  func (n *noopFreezer) IsFrozen(context.Context, string) (bool, error) { return false, nil }
  func (n *noopFreezer) HasLiveHashWithdraw(context.Context, string) (bool, error) {
	  return false, nil
   }
