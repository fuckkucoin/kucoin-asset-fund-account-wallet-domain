package haltclear

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

// ---- 鉴权接口 -------------------------------------------------------
// ActorChecker 运营鉴权（与 custodyhalt.ActorChecker 同接口）。
type ActorChecker interface {
	ID() string
	CheckOps() error
}

// LeaderChecker 选主约束。

type LeaderChecker interface {
	RequireLeader() error
}

// ---- Mount ----------------------------------------------------------
// Mount 挂载 haltclear 运营 HTTP 路由；所有接口须先通过 actorFn 鉴权。
// 功能：清闸四眼（propose/approve/reject/proof）、强制清闸升级、误清查询、再熔断调度与确认。
// 不写级联冻结；不写对账工单；clear 仅人工。
   func Mount(
	   v1 *gin.RouterGroup,
	   svc *Service,
	  leader LeaderChecker,
	   actorFn func(*gin.Context) (ActorChecker,
		   bool),
) {
	if v1 == nil || svc == nil {
		return
	}
	if actorFn == nil {
		actorFn = func(*gin.Context) (ActorChecker, bool) { return nil, false }
	}
	hc := v1.Group("/ops/custody/halt-clear")
	// ---- 四眼清闸 ----
	// POST /ops/custody/halt-clear/plans — 提交清闸提案
	hc.POST("/plans", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		   if !ok {
			  return
		   }
		   if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  requireLeader(c, leader)
		   if c.IsAborted() {
			   return
		  }
		  var body struct {
			   PlanID string `json:"plan_id"`
			   Scope  string `json:"scope"`
			   Chain string `json:"chain"`
			  Asset     string `json:"asset"`
			   Reason    string `json:"reason"`
			  OpenDiffs int    `json:"open_diff_count"`
			   DiffSummary string `json:"diff_summary"`
			  FreezeWarRoomPlanID string `json:"freeze_war_room_plan_id"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			  return
		  }
		  if body.PlanID == "" {
			   body.PlanID = newPlanIDHTTP(actor.ID())
		  }
		   plan, err := svc.ProposePlan(c.Request.Context(), ProposePlanReq{
			   PlanID:   body.PlanID,
			  Scope:    body.Scope,
			   Chain:    body.Chain,
			   Asset:    body.Asset,
			   Proposer: actor.ID(),
			   Reason:    body.Reason,
			  OpenDiffs: body.OpenDiffs,
			  DiffSummary: body.DiffSummary,
			  FreezeWarRoomPlanID: body.FreezeWarRoomPlanID,
		  })
		  if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"plan": plan, "host": "custody-boundary-lab.kucoin.net"})
	   })
	// GET /ops/custody/halt-clear/plans — 列出提案
	hc.GET("/plans", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		  if !ok {
			   return
		  }
		   if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		  plans, err := svc.ListPlans(c.Request.Context(), limit)
		  if err != nil {
			  c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"plans": plans, "count": len(plans), "host": "custody-boundary-lab.kucoin.net"})
	  })
	// GET /ops/custody/halt-clear/plans/:plan_id — 查单个提案
	hc.GET("/plans/:plan_id", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		  if !ok {
			   return
		   }
		  if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  plan, err := svc.GetPlan(c.Request.Context(), c.Param("plan_id"))
		  if err != nil {
			  if isNotFound(err) {
				  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  } else {
				  c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			   }
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"plan": plan})
	   })
	// POST /ops/custody/halt-clear/plans/:plan_id/approve1 — 第一签
	hc.POST("/plans/:plan_id/approve1", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			  return
		   }
		   if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  requireLeader(c, leader)
		  if c.IsAborted() {
			  return
		  }
		   plan, err := svc.FirstApprovePlan(c.Request.Context(), c.Param("plan_id"), actor.ID())
		  if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"plan": plan})
	  })
	// POST /ops/custody/halt-clear/plans/:plan_id/approve2 — 第二签（完成四眼，返回 CloseProof）
	hc.POST("/plans/:plan_id/approve2", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			   return
		  }
		  if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  requireLeader(c, leader)
		  if c.IsAborted() {
			  return
		   }
		  var body struct {
			   OpenDiffs int `json:"open_diff_count"`
			  DiffSummary string `json:"diff_summary"`
		  }
		   _ = c.ShouldBindJSON(&body)
		   proof, err := svc.SecondApprovePlan(c.Request.Context(), c.Param("plan_id"), actor.ID(), body.OpenDiffs, body.DiffSummary)
		  if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"proof": proof, "host": "custody-boundary-lab.kucoin.net"})
	  })
	// POST /ops/custody/halt-clear/plans/:plan_id/reject — 否决提案
	hc.POST("/plans/:plan_id/reject", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		  if !ok {
			  return
		   }
		   if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  Reason string `json:"reason"`
		  }
		  _ = c.ShouldBindJSON(&body)
		  plan, err := svc.RejectPlan(c.Request.Context(), c.Param("plan_id"), actor.ID(), body.Reason)
		  if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"plan": plan})
	  })
	// GET /ops/custody/halt-clear/proofs/:proof_id — 读取闭合证明
	hc.GET("/proofs/:proof_id", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			  return
		   }
		  if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   proof, err := svc.GetProof(c.Request.Context(), c.Param("proof_id"))
		  if err != nil {
			  if isNotFound(err) {
				   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  } else {
				   c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			   }
			   return
		  }
		  valid := custodyhalt.VerifyProof(*proof)
		   c.JSON(http.StatusOK, gin.H{"proof": proof, "checksum_valid": valid == nil, "host": "custody-boundary-lab.kucoin.net"})
	  })
	// ---- 强制清闸升级 ----
	// POST /ops/custody/halt-clear/forced — 强制清闸（必须携带 reason_code）
	hc.POST("/forced", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		  if !ok {
			   return
		   }
		   if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  requireLeader(c, leader)
		  if c.IsAborted() {
			   return
		   }
		  var body struct {
			   Scope string `json:"scope"`
			  Chain string `json:"chain"`
			   Asset string `json:"asset"`
			   Reason string `json:"reason"`
			   ReasonCode string `json:"reason_code"`
			   PostTicketID string `json:"post_ticket_id"`
			   OpenDiffCount int `json:"open_diff_count"`
			   DiffKeys     []string `json:"diff_keys"`
			  FlagSnapshot string   `json:"flag_snapshot"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			   return
		   }
		   rec, mc, err := svc.ForcedClear(c.Request.Context(), ForcedClearReq{
			   Scope: body.Scope,
			  Chain: body.Chain,
			   Asset: body.Asset,
			  Actor:      actor.ID(),
			   Reason:     body.Reason,
			  ReasonCode: body.ReasonCode,
			   PostTicketID: body.PostTicketID,
			  OpenDiffCount: body.OpenDiffCount,
			   DiffKeys:      body.DiffKeys,
			  FlagSnapshot:  body.FlagSnapshot,
		   })
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(),
				   "hint": "reason_code required; set post_ticket_id for other reason"})
			  return
		  }
		  resp := gin.H{"audit_record": rec, "host": "custody-boundary-lab.kucoin.net"}
		   if mc != nil {
			   resp["misclear_case"] = mc
			  resp["misclear_risk"] = true
		   }
		  c.JSON(http.StatusOK, resp)
	  })
	// GET /ops/custody/halt-clear/forced-audits — 列出强制清闸审计
	hc.GET("/forced-audits", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		  if !ok {
			   return
		   }
		   if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		   recs := svc.forcedAudit.QueryForced(c.Request.Context(),
			   c.Query("actor"), c.Query("scope"), c.Query("reason_code"), limit)
		  c.JSON(http.StatusOK, gin.H{"records": recs, "count": len(recs), "host": "custody-boundary-lab.kucoin.net"})
	   })
	// ---- 误清查询 ----
	// GET /ops/custody/halt-clear/cases — 列出误清案例
	hc.GET("/cases", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		  if !ok {
			  return
		   }
		  if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		  f := CaseFilter{
			   Scope: c.Query("scope"),
			  Chain: strings.ToUpper(c.Query("chain")),
			  Asset: strings.ToUpper(c.Query("asset")),
			   State: MisClearCaseState(c.Query("state")),
			   Trigger: MisClearTrigger(c.Query("trigger")),
			  Limit:   limit,
		   }
		   cases, err := svc.ListMisClearCases(c.Request.Context(), f)
		   if err != nil {
			  c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"cases": cases, "count": len(cases), "host": "custody-boundary-lab.kucoin.net"})
	   })
	// GET /ops/custody/halt-clear/cases/:misclear_id — 查单个案例
	hc.GET("/cases/:misclear_id", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			  return
		   }
		  if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   mc, err := svc.GetMisClearCase(c.Request.Context(), c.Param("misclear_id"))
		  if err != nil {
			   if isNotFound(err) {
				   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  } else {
				  c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			   }
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"case": mc})
	  })
	// ---- 再熔断调度 ----
	// POST /ops/custody/halt-clear/cases/:misclear_id/reengage — 调度再熔断腿
	hc.POST("/cases/:misclear_id/reengage", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		  if !ok {
			  return
		   }
		   if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  requireLeader(c, leader)
		   if c.IsAborted() {
			  return
		  }
		   var body struct {
			   Reason string `json:"reason"`
			   IsAuto bool   `json:"is_auto"`
		   }
		   _ = c.ShouldBindJSON(&body)
		   leg, err := svc.ScheduleReEngage(c.Request.Context(), ScheduleReEngageReq{
			   MisclearID: c.Param("misclear_id"),
			   ActorID:    actor.ID(),
			   Reason:     body.Reason,
			   IsAuto:     body.IsAuto,
		   })
		  if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"leg": leg, "host": "custody-boundary-lab.kucoin.net"})
	   })
	// POST /ops/custody/halt-clear/legs/:leg_id/confirm — 人工确认执行再熔断腿
	hc.POST("/legs/:leg_id/confirm", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		  if !ok {
			   return
		  }
		  if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   requireLeader(c, leader)
		   if c.IsAborted() {
			  return
		  }
		   proof, err := svc.ConfirmReEngage(c.Request.Context(), c.Param("leg_id"))
		  if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"rollback_proof": proof, "proof_valid": VerifyRollbackProof(proof), "host": "custody-boundary-lab.kucoin.net"})
	  })
	// GET /ops/custody/halt-clear/cases/:misclear_id/legs — 列出案例的全部腿
	hc.GET("/cases/:misclear_id/legs", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			  return
		   }
		  if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		   legs, err := svc.ListLegs(c.Request.Context(), c.Param("misclear_id"), limit)
		   if err != nil {
			   c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"legs": legs, "count": len(legs)})
	   })
	// POST /ops/custody/halt-clear/cases/:misclear_id/close — 人工关闭案例
	hc.POST("/cases/:misclear_id/close", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			  return
		   }
		   if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Note string `json:"note"`
		   }
		   _ = c.ShouldBindJSON(&body)
		   if err := svc.CloseCase(c.Request.Context(), c.Param("misclear_id"), actor.ID(), body.Note); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"closed": true, "closed_by": actor.ID(), "closed_at": time.Now().UTC()})
	   })
}

// ---- 内部辅助 -------------------------------------------------------

  func requireLeader(c *gin.Context, leader LeaderChecker) {
	  if leader == nil {
		  return
	  }
	   if err := leader.RequireLeader(); err != nil {
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		   c.Abort()
	  }
  }

  func isNotFound(err error) bool {
	  return strings.Contains(err.Error(), "not found")
  }

   func newPlanIDHTTP(actor string) string {
	   return fmt.Sprintf("plan:%d:%s", time.Now().UnixNano(), strings.TrimSpace(actor))
  }
