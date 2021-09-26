// 功能：p2pabandon 包内 http 实现。
// P2P 弃单收口——HTTP API 注册与处理器。
// API 清单：
// GET /p2p/abandon/plan 按 plan_id 或 order_id 查询弃单计划
// GET /p2p/abandon/plans 枚举所有计划（管理员）
// GET /p2p/abandon/token/summary 查询计划互斥令牌汇总
// GET /p2p/abandon/age 查询计划超龄摘要
// GET /p2p/abandon/proof 查询收口证明
// POST /p2p/abandon/force 强制弃单（双人确认）
// POST /p2p/abandon/conflict/resolve 仲裁冲突（desk 双人）
// POST /p2p/abandon/validate-close 收口校验（标 closed 前必调）
// GET /p2p/abandon/ages/escalated 查询所有 escalated 超龄记录（desk）
// POST /p2p/abandon/age/resolve 标记超龄记录已处置
// 鉴权约定：
// 查询接口需 ActQueryRead；写入接口需 ActAccountMove；强制弃单额外做双人校验（在业务层）。

// 作者：KnoxRay
// 日期：2021.09.19

package p2pabandon

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
)

// Register 注册 P2P 弃单收口 HTTP API 到 Gin 路由组。

   func Register(v1 *gin.RouterGroup, svc *Service) {
	   if v1 == nil || svc == nil {
		   return
	  }
	   // ── 查询接口 ──
	   v1.GET("/p2p/abandon/plan", handleGetPlan(svc))
	  v1.GET("/p2p/abandon/plans", handleListPlans(svc))
	   v1.GET("/p2p/abandon/token/summary", handleTokenSummary(svc))
	  v1.GET("/p2p/abandon/age", handleGetAge(svc))
	   v1.GET("/p2p/abandon/proof", handleGetProof(svc))
	   v1.GET("/p2p/abandon/ages/escalated", handleListEscalated(svc))
	   // ── 写入接口 ──
	  v1.POST("/p2p/abandon/force", handleForceAbandon(svc))
	   v1.POST("/p2p/abandon/conflict/resolve", handleConflictResolve(svc))
	  v1.POST("/p2p/abandon/validate-close", handleValidateClose(svc))
	  v1.POST("/p2p/abandon/age/resolve", handleResolveAge(svc))
   }

// ─────────────────────────── 查询处理器 ───────────────────────────

   func handleGetPlan(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   planID := c.Query("plan_id")
		   orderID := c.Query("order_id")
		  var plan AbandonPlan
		  var err error
		  if planID != "" {
			  plan,
				   err = svc.GetPlan(planID)
		  } else if orderID != "" {
			  plan, err = svc.GetPlanByOrder(orderID)
		  } else {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id or order_id required"})
			   return
		  }
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"plan": plan})
	   }
   }

  func handleListPlans(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   statusFilter := c.Query("status")
		  var plans []AbandonPlan
		   if statusFilter == "" {
			   plans = svc.ListAllPlans()
		  } else {
			   all := svc.ListAllPlans()
			   for _, p := range all {
				   if string(p.Status) == statusFilter {
					   plans = append(plans, p)
				  }
			   }
		   }
		   if plans == nil {
			  plans = []AbandonPlan{}
		   }
		   c.JSON(http.StatusOK, gin.H{"plans": plans, "count": len(plans)})
	   }
  }

   func handleTokenSummary(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   planID := c.Query("plan_id")
		   if planID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id required"})
			  return
		   }
		  sum, err := svc.GetTokenSummary(planID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"summary": sum})
	   }
   }

   func handleGetAge(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  planID := c.Query("plan_id")
		  ageID := c.Query("age_id")
		   if planID != "" {
			  sum, err := svc.GetHoldAgeSummary(planID)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			  c.JSON(http.StatusOK, gin.H{"age_summary": sum})
			  return
		   }
		   if ageID != "" {
			  age, err := svc.GetAge(ageID)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			   }
			  c.JSON(http.StatusOK, gin.H{"age": age})
			  return
		   }
		  c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id or age_id required"})
	   }
  }

  func handleGetProof(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  planID := c.Query("plan_id")
		   if planID == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id required"})
			  return
		  }
		   proof, err := svc.GetProofByPlan(planID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"proof": proof})
	   }
   }

  func handleListEscalated(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  ages := svc.ListEscalatedAges()
		   if ages == nil {
			   ages = []HoldAge{}
		  }
		   c.JSON(http.StatusOK, gin.H{"escalated_ages": ages, "count": len(ages)})
	  }
   }

// ─────────────────────────── 写入处理器 ───────────────────────────

   func handleForceAbandon(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   var body struct {
			   PlanID    string `json:"plan_id"`
			  Operator1 string `json:"operator1"`
			   Operator2 string `json:"operator2"`
			  Reason string `json:"reason"`
			   AgeRecordID string `json:"age_record_id,omitempty"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   plan, proof, err := svc.ForceAbandon(ForceAbandonReq{
			  PlanID:    body.PlanID,
			  Operator1: body.Operator1,
			   Operator2: body.Operator2,
			   Reason: body.Reason,
			  AgeRecordID: body.AgeRecordID,
		  })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"plan": plan, "proof": proof})
	  }
  }

   func handleConflictResolve(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  var body struct {
			  PlanID string `json:"plan_id"`
			  Operator1 string `json:"operator1"`
			   Operator2        string `json:"operator2"`
			  Outcome          string `json:"outcome"` // "released" | "filled"
			  CommittedTokenID string `json:"committed_token_id,omitempty"`
			   LedgerRef string `json:"ledger_ref,omitempty"`
			  HoldRef string `json:"hold_ref,omitempty"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  outcome := CloseOutcome(body.Outcome)
		   if !outcome.Valid() || outcome == OutcomeConflict {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "outcome must be released or filled"})
			   return
		  }
		  proof, err := svc.GenerateConflictResolutionProof(
			   body.PlanID,
			   body.Operator1, body.Operator2, outcome,
			  body.CommittedTokenID, body.LedgerRef, body.HoldRef,
		   )
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  plan, _ := svc.GetPlan(body.PlanID)
		   c.JSON(http.StatusOK, gin.H{"proof": proof, "plan": plan})
	  }
   }

   func handleValidateClose(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   var body struct {
			   PlanID string `json:"plan_id"`
			   OrderID string `json:"order_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   resp, err := svc.ValidateClose(ValidateCloseReq{
			  PlanID: body.PlanID,
			   OrderID: body.OrderID,
		   })
		  if err != nil && err != ErrNoProof && err != ErrProofTamper {
			  mw.WriteProblem(c, err)
			  return
		   }
		   statusCode := http.StatusOK
		   if !resp.CanClose {
			  statusCode = http.StatusConflict
		   }
		   c.JSON(statusCode, gin.H{"validate_close": resp})
	   }
   }

  func handleResolveAge(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  var body struct {
			   AgeID string `json:"age_id"`
			  ResolvedBy string `json:"resolved_by"`
			   ResolveNote string `json:"resolve_note"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   age, err := svc.ResolveAge(ResolveAgeReq{
			   AgeID: body.AgeID,
			   ResolvedBy: body.ResolvedBy,
			   ResolveNote: body.ResolveNote,
		   })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"age": age})
	   }
  }
