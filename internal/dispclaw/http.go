package dispclaw

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 挂载 dispute-claw 路由到 v1 分组。
// 中 svc 由带 SQL 后端的适配器注入；此处直接使用内存后端，便于集成。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  _ = rt
	  // ── 生成资金计划 ──────────────────────────────────────────────────
	   v1.POST("/dispute-claw/plan/generate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  CaseID   string `json:"case_id"`
			  PlanType string `json:"plan_type"`
			  Actor string `json:"actor"`
			   RailBreakID string `json:"rail_break_id,omitempty"`
			  IdempotencyKey string `json:"idempotency_key"`
			  // Case 字段由调用方从 disputedesk 查出后传入，避免本包依赖 disputedesk。
			   CaseUserID string `json:"case_user_id"`
			   CaseChannel string `json:"case_channel"`
			   CaseAsset string `json:"case_asset"`
			  CaseAmount string `json:"case_amount"`
			   OpenedBy string `json:"opened_by"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   req := GeneratePlanReq{
			  CaseID: strings.TrimSpace(body.CaseID),
			  PlanType:       PlanType(strings.TrimSpace(body.PlanType)),
			   Actor:          body.Actor,
			   RailBreakID:    strings.TrimSpace(body.RailBreakID),
			   IdempotencyKey: strings.TrimSpace(body.IdempotencyKey),
		  }
		  plan,
			  legs, err := svc.GeneratePlan(req,
			  body.CaseUserID, body.CaseChannel, body.CaseAsset, body.CaseAmount, body.OpenedBy)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"plan": plan, "legs": legs})
	  })
	   // ── 执行追偿腿（四眼） ────────────────────────────────────────────
	   v1.POST("/dispute-claw/leg/execute", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body ExecuteLegReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   leg, err := svc.ExecuteLeg(body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, leg)
	   })
	   // ── 跳过追偿腿 ───────────────────────────────────────────────────
	   v1.POST("/dispute-claw/leg/skip", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  LegID string `json:"leg_id"`
			  Reason string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  leg, err := svc.SkipLeg(body.LegID, body.Reason)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, leg)
	   })
	   // ── 检查收口闸 ───────────────────────────────────────────────────
	  v1.POST("/dispute-claw/gate/check", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body CloseGateCheckReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		  gate, err := svc.CheckCloseGate(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gate)
	  })
	   // ── 管理员特批豁免收口闸 ─────────────────────────────────────────
	  v1.POST("/dispute-claw/gate/bypass", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body BypassGateReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   gate, err := svc.BypassCloseGate(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gate)
	   })
	   // ── 生成收口证明 ─────────────────────────────────────────────────
	   v1.POST("/dispute-claw/proof/generate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  CaseID string `json:"case_id"`
			   Actor  string `json:"actor"`
			   Decision string `json:"decision"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   proof,
			  err := svc.GenerateCloseProof(body.CaseID, body.Actor, body.Decision)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, proof)
	  })
	  // ── 触发收口证明巡检（运维诊断用） ──────────────────────────────
	  v1.POST("/dispute-claw/proof/patrol", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   CaseID string `json:"case_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   proof, err := svc.PatrolProof(strings.TrimSpace(body.CaseID))
		  if err != nil && err != ErrProofTamper {
			  mw.WriteProblem(c, err)
			  return
		   }
		  tampered := err == ErrProofTamper
		   c.JSON(200, gin.H{"proof": proof, "tamper_detected": tampered})
	  })
	   // ── 中止资金计划 ─────────────────────────────────────────────────
	   v1.POST("/dispute-claw/plan/abort", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body AbortPlanReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		  plan, err := svc.AbortPlan(body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, plan)
	   })
	  // ── 查询 FundPlan ────────────────────────────────────────────────
	  v1.GET("/dispute-claw/plan", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  caseID := strings.TrimSpace(c.Query("case_id"))
		  planID := strings.TrimSpace(c.Query("plan_id"))
		   var plan FundPlan
		   var err error
		   if caseID != "" {
			   plan, err = svc.GetPlanByCase(caseID)
		  } else if planID != "" {
			  plan, err = svc.GetPlan(planID)
		  } else {
			  c.JSON(400, gin.H{"error": "case_id or plan_id required"})
			  return
		  }
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, plan)
	   })
	   // ── 查询追偿腿列表 ───────────────────────────────────────────────
	   v1.GET("/dispute-claw/legs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   planID := strings.TrimSpace(c.Query("plan_id"))
		  if planID == "" {
			   c.JSON(400, gin.H{"error": "plan_id required"})
			   return
		   }
		  c.JSON(200, gin.H{"legs": svc.ListLegs(planID)})
	  })
	   // ── 查询收口闸 ───────────────────────────────────────────────────
	   v1.GET("/dispute-claw/gate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   caseID := strings.TrimSpace(c.Query("case_id"))
		   if caseID == "" {
			  c.JSON(400, gin.H{"error": "case_id required"})
			  return
		  }
		  gate, err := svc.GetGateByCaseID(caseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gate)
	  })
	  // ── 查询收口证明 ─────────────────────────────────────────────────
	   v1.GET("/dispute-claw/proof", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   caseID := strings.TrimSpace(c.Query("case_id"))
		  if caseID == "" {
			   c.JSON(400, gin.H{"error": "case_id required"})
			  return
		   }
		  proof, err := svc.GetProofByCaseID(caseID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, proof)
	  })
  }
