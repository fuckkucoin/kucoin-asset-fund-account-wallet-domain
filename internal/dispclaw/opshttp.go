package dispclaw

import (
	"fmt"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterOps 挂载运营台专用路由到 v1 分组。
// 路由前缀 /ops/dispute-claw/。

   func RegisterOps(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  if svc == nil {
		   svc = New()
	  }
	  _ = rt
	  // ── 批量查询所有计划（分页） ─────────────────────────────────────
	   v1.GET("/ops/dispute-claw/plans", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  status := strings.TrimSpace(c.Query("status"))
		  limitStr := c.DefaultQuery("limit", "50")
		   limit, _ := strconv.Atoi(limitStr)
		  if limit <= 0 || limit > 200 {
			  limit = 50
		  }
		   all := svc.ListAllPlans()
		  var filtered []FundPlan
		  for _, p := range all {
			  if status != "" && string(p.Status) != status {
				   continue
			  }
			  filtered = append(filtered, p)
			   if len(filtered) >= limit {
				  break
			   }
		   }
		  c.JSON(200, gin.H{
			  "plans": filtered,
			   "count": len(filtered),
		  })
	   })
	   // ── 批量查询所有收口证明 ─────────────────────────────────────────
	  v1.GET("/ops/dispute-claw/proofs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  all := svc.ListAllProofs()
		  c.JSON(200, gin.H{
			   "proofs": all,
			  "count": len(all),
		  })
	   })
	   // ── 指标概览（运营台仪表盘） ─────────────────────────────────────
	   v1.GET("/ops/dispute-claw/stats", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{
			  "counts": svc.Counts(),
			   "host":   "kyt.kucoin.net",
		   })
	  })
	  // ── 触发全量证明巡检（批量） ──────────────────────────────────────
	   v1.POST("/ops/dispute-claw/proof/patrol-all", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   proofs := svc.ListAllProofs()
		   results := make([]gin.H, 0, len(proofs))
		   tamperedCount := 0
		   for _, proof := range proofs {
			   _, err := svc.PatrolProof(proof.CaseID)
			   tampered := err == ErrProofTamper
			  if tampered {
				   tamperedCount++
			   }
			   results = append(results, gin.H{
				  "case_id":  proof.CaseID,
				  "proof_id": proof.ID,
				   "tamper_detected": tampered,
			   })
		   }
		  c.JSON(200, gin.H{
			  "total": len(proofs),
			   "tampered_count": tamperedCount,
			   "results": results,
		   })
	  })
	   // ── 查询超期未完成计划列表 ────────────────────────────────────────
	   v1.GET("/ops/dispute-claw/plans/timeout", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  threshHoursStr := c.DefaultQuery("threshold_hours", "72")
		   threshHours, _ := strconv.ParseFloat(threshHoursStr, 64)
		   if threshHours <= 0 {
			   threshHours = 72
		  }
		   all := svc.ListAllPlans()
		  var timedOut []gin.H
		  for _, p := range all {
			   if p.Status.Terminal() {
				  continue
			   }
			  // threshold 由调用方指定（中可用较小值）。
			   timedOut = append(timedOut, gin.H{
				   "plan_id": p.ID,
				  "case_id": p.CaseID,
				   "status":    string(p.Status),
				   "plan_type": string(p.PlanType),
				  "opened_by": p.OpenedBy,
				   "age_note":  fmt.Sprintf("plan open since %s", p.CreatedAt.Format("2006-01-02T15:04:05Z")),
			  })
		   }
		   c.JSON(200, gin.H{
			  "timeout_plans": timedOut,
			   "threshold_h":   threshHours,
			  "count": len(timedOut),
		   })
	   })
	  // ── 单个计划完整快照（调试用） ────────────────────────────────────
	  v1.GET("/ops/dispute-claw/plan/snapshot", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   caseID := strings.TrimSpace(c.Query("case_id"))
		   if caseID == "" {
			  c.JSON(400, gin.H{"error": "case_id required"})
			  return
		   }
		   plan,
			   err := svc.GetPlanByCase(caseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  legs := svc.ListLegs(plan.ID)
		  gate, _ := svc.GetGateByCaseID(caseID)
		  proof, _ := svc.GetProofByCaseID(caseID)
		   c.JSON(200, gin.H{
			   "plan": plan,
			   "legs": legs,
			  "gate":  gate,
			  "proof": proof,
		  })
	   })
  }
