package amlhold

import (
	"net/http"
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/opsauth"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 注册所有 amlhold HTTP 路由。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	  _ = rt
	   // ── 开启 hold ──────────────────────────────────────────────────────────
	   v1.POST("/aml/holds", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActAMLOpen, perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   CaseID string `json:"case_id"`
			  UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			  Amount      string `json:"amount"`
			   SLALimitSec int64  `json:"sla_limit_sec"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  h, err := svc.OpenHold(body.CaseID, body.UserID, body.Asset, body.Amount, actor.ID, body.SLALimitSec)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"hold": h})
	  })
	  // ── 查询单条 hold ──────────────────────────────────────────────────────
	  v1.GET("/aml/holds/:id", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  h, err := svc.GetHold(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"hold": h})
	  })
	   // ── 列举 hold（按 case_id 或 user_id）────────────────────────────────
	  v1.GET("/aml/holds", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  caseID := c.Query("case_id")
		  userID := c.Query("user_id")
		  openOnly := c.Query("open_only") == "true"
		   if caseID != "" {
			   holds := svc.ListByCase(caseID,
				   openOnly)
			   c.JSON(http.StatusOK, gin.H{"holds": holds, "count": len(holds)})
			  return
		   }
		  if userID != "" {
			   holds := svc.ListByUser(userID, HoldFilter{OpenOnly: openOnly, Limit: 100})
			  c.JSON(http.StatusOK, gin.H{"holds": holds, "count": len(holds)})
			   return
		  }
		   c.JSON(http.StatusBadRequest, gin.H{"error": "case_id or user_id required"})
	   })
	   // ── 手动推进老化 ───────────────────────────────────────────────────────
	  v1.POST("/aml/holds/:id/advance-aging", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActAMLClose, perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  Reason string `json:"reason"`
			   Epoch int64  `json:"epoch"`
			  Token string `json:"token"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  h, ev, err := svc.AdvanceAging(c.Param("id"), actor.ID, body.Reason, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"hold": h, "event": ev})
	  })
	   // ── 申请解冻 ──────────────────────────────────────────────────────────
	  v1.POST("/aml/holds/:id/request-release", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := opsauth.RequireAny(actor, perm.ActAMLClose, perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   CaseID string `json:"case_id"`
			  Reason string `json:"reason"`
			   CaseCloserActor string `json:"case_closer_actor"` // 调用方提供，用于互斥检查
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   t, err := svc.RequestRelease(c.Param("id"), body.CaseID, actor.ID, body.Reason, body.CaseCloserActor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"ticket": t})
	   })
	  // ── 列出 hold 解冻申请单 ───────────────────────────────────────────────
	  v1.GET("/aml/holds/:id/tickets", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   tickets := svc.TicketsByHold(c.Param("id"))
		  c.JSON(http.StatusOK, gin.H{"tickets": tickets, "count": len(tickets)})
	   })
	  // ── 批准解冻 ──────────────────────────────────────────────────────────
	   v1.POST("/aml/holds/tickets/:ticket_id/approve", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActAMLClose, perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Epoch int64 `json:"epoch"`
			  Token string `json:"token"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  t, h, err := svc.ApproveRelease(c.Param("ticket_id"), actor.ID, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"ticket": t, "hold": h})
	  })
	  // ── 拒绝解冻 ──────────────────────────────────────────────────────────
	  v1.POST("/aml/holds/tickets/:ticket_id/reject", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := opsauth.RequireAny(actor, perm.ActAMLClose, perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Reason string `json:"reason"`
			  Epoch  int64  `json:"epoch"`
			  Token string `json:"token"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  t, err := svc.RejectRelease(c.Param("ticket_id"), actor.ID, body.Reason, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"ticket": t})
	  })
	   // ── 查询 hold 证明 ─────────────────────────────────────────────────────
	   v1.GET("/aml/holds/:id/proofs", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  proofs := svc.Proofs(c.Param("id"))
		   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
	   })
	   // ── 查询 hold 缺口单 ───────────────────────────────────────────────────
	   v1.GET("/aml/holds/:id/gaps", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   gaps := svc.GapTickets(c.Param("id"))
		   c.JSON(http.StatusOK, gin.H{"gaps": gaps, "count": len(gaps)})
	  })
	   // ── 导出 Case 下所有 hold 证明 ─────────────────────────────────────────
	  v1.GET("/aml/cases/:case_id/hold-proofs", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   proofs, err := svc.ExportProofsForCase(c.Param("case_id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
	   })
	  // ── SLA 状态报告（只读）───────────────────────────────────────────────
	   v1.GET("/aml/holds/sla-scan", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  report := svc.SLAScan(time.Now().UTC())
		  c.JSON(http.StatusOK, gin.H{"reports": report, "count": len(report), "scanned_at": time.Now().UTC()})
	   })
	   // ── 管理员强制终止 hold ───────────────────────────────────────────────
	   v1.POST("/aml/holds/:id/abort", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := opsauth.RequireAny(actor, perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Reason string `json:"reason"`
			   Epoch  int64  `json:"epoch"`
			  Token string `json:"token"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  h, err := svc.AbortHold(c.Param("id"), actor.ID, body.Reason, body.Epoch, body.Token)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"hold": h})
	  })
	   // ── 统计 ───────────────────────────────────────────────────────────────
	   v1.GET("/aml/holds/stats", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   stats := svc.Stats()
		   c.JSON(http.StatusOK, stats)
	  })
	   // ── 漂移巡检触发（ops）────────────────────────────────────────────────
	  v1.POST("/aml/holds/drift-check", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := opsauth.RequireAny(actor, perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  gaps := svc.CheckDrift()
		   c.JSON(http.StatusOK, gin.H{"gaps": gaps, "count": len(gaps)})
	   })
	  // ── 运行一次老化扫描（ops）────────────────────────────────────────────
	  v1.POST("/aml/holds/run-aging-scan", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := opsauth.RequireAny(actor, perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   result := svc.RunAgingScan()
		  c.JSON(http.StatusOK, result)
	   })
	   // ── 解冻申请单详情 ─────────────────────────────────────────────────────
	   v1.GET("/aml/holds/tickets/:ticket_id", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   t, err := svc.GetTicket(c.Param("ticket_id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"ticket": t})
	  })
	  // ── 解决缺口单 ────────────────────────────────────────────────────────
	   v1.POST("/aml/holds/gaps/:gap_id/resolve", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := opsauth.RequireAny(actor, perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  if err := svc.ResolveGap(c.Param("gap_id"), actor.ID); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"resolved": true})
	   })
	   // ── 到期需升级的 hold 列表 ────────────────────────────────────────────
	  v1.GET("/aml/holds/due", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  atStr := c.Query("at")
		   var at time.Time
		  if atStr != "" {
			  ts, err := strconv.ParseInt(atStr, 10, 64)
			   if err == nil {
				  at = time.Unix(ts, 0).UTC()
			  }
		   }
		  if at.IsZero() {
			  at = time.Now().UTC()
		  }
		  holds := svc.DueHolds(at)
		   c.JSON(http.StatusOK, gin.H{"holds": holds, "count": len(holds), "at": at})
	   })
   }
