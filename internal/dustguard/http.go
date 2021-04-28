package dustguard

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 挂载防护路由到 v1 路由组。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime, svc *GuardService) {
	   if svc == nil {
		  return
	  }
	  g := v1.Group("/dustguard")
	  g.Use(requireOpsAdmin)
	   // ─── 攻击分值 ───────────────────────────────────────────────────────
	   g.GET("/scores", func(c *gin.Context) {
		   chain := strings.ToUpper(strings.TrimSpace(c.Query("chain")))
		   lvl := AttackLevel(strings.ToLower(c.Query("min_level")))
		  if lvl == "" {
			   lvl = LevelSuspicious
		  }
		   scores, err := svc.detector.store.ListScores(c.Request.Context(), chain, lvl)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"scores": scores})
	  })
	  // ─── 隔离案 ─────────────────────────────────────────────────────────
	   g.GET("/quarantine", func(c *gin.Context) {
		  state := QuarantineState(c.Query("state"))
		   chain := c.Query("chain")
		   cases, err := svc.fsm.ListCases(c.Request.Context(), state, chain)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"cases": cases, "count": len(cases)})
	  })
	   g.POST("/quarantine", func(c *gin.Context) {
		   var body struct {
			   Chain string `json:"chain"`
			  Address string `json:"address"`
			  UserID string `json:"user_id"`
			   Reason string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  actor := actorFromCtx(c)
		   score,
			   _ := svc.detector.Score(c.Request.Context(), body.Chain, body.Address)
		   cs,
			   err := svc.fsm.Open(c.Request.Context(), body.Chain, body.Address, body.UserID, actor, score, body.Reason)
		  if err != nil && err != ErrAlreadyQuarantined {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"case": cs})
	   })
	  g.POST("/quarantine/:id/advance", func(c *gin.Context) {
		  actor := actorFromCtx(c)
		  cs, err := svc.fsm.Advance(c.Request.Context(), c.Param("id"), actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"case": cs})
	   })
	   g.POST("/quarantine/:id/release", func(c *gin.Context) {
		   actor := actorFromCtx(c)
		  cs, err := svc.fsm.Release(c.Request.Context(), c.Param("id"), actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"case": cs})
	   })
	   g.POST("/quarantine/:id/burn", func(c *gin.Context) {
		  var body struct {
			  Reason string `json:"reason"`
		  }
		  _ = mw.BindJSON(c, &body)
		   actor := actorFromCtx(c)
		   cs, err := svc.fsm.Burn(c.Request.Context(), c.Param("id"), actor, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"case": cs})
	   })
	  // ─── 轮换计划 ───────────────────────────────────────────────────────
	  g.GET("/rotate", func(c *gin.Context) {
		  state := RotateState(c.Query("state"))
		   chain := c.Query("chain")
		   plans, err := svc.rotator.ListPlans(c.Request.Context(), state, chain)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"plans": plans, "count": len(plans)})
	  })
	   g.POST("/rotate", func(c *gin.Context) {
		   var body struct {
			  Chain string `json:"chain"`
			   OldAddress string `json:"old_address"`
			   NewAddress string `json:"new_address"`
			  UserID     string `json:"user_id"`
			  Reason string `json:"reason"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  actor := actorFromCtx(c)
		   plan,
			  err := svc.rotator.OpenOpsRotate(c.Request.Context(),
			  body.Chain,
			  body.OldAddress, body.NewAddress, body.UserID, actor, body.Reason)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"plan": plan})
	  })
	  g.POST("/rotate/:id/approve", func(c *gin.Context) {
		   var body struct {
			  Allow bool `json:"allow"`
			  Epoch int64 `json:"epoch"`
			  Token string `json:"token"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  actor := actorFromCtx(c)
		  plan, err := svc.rotator.ApproveOpsRotate(c.Request.Context(),
			  c.Param("id"), actor, body.Allow, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"plan": plan})
	  })
	   g.POST("/rotate/:id/execute", func(c *gin.Context) {
		  var body struct {
			   Epoch int64 `json:"epoch"`
			   Token string `json:"token"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  actor := actorFromCtx(c)
		   result, err := svc.rotator.ExecuteRotate(c.Request.Context(), c.Param("id"), actor, body.Epoch, body.Token)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"result": result})
	  })
	   g.POST("/rotate/:id/abort", func(c *gin.Context) {
		   var body struct {
			  Reason string `json:"reason"`
		   }
		  _ = mw.BindJSON(c, &body)
		  actor := actorFromCtx(c)
		   plan, err := svc.rotator.AbortPlan(c.Request.Context(), c.Param("id"), actor, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"plan": plan})
	   })
	   // ─── 策略管理 ───────────────────────────────────────────────────────
	  g.GET("/policies", func(c *gin.Context) {
		   chain := c.Query("chain")
		   pols, err := svc.policyMgr.List(c.Request.Context(), chain)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"policies": pols, "count": len(pols)})
	   })
	   g.POST("/policies", func(c *gin.Context) {
		  var body PoolGuardPolicy
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   actor := actorFromCtx(c)
		   pol, err := svc.policyMgr.Create(c.Request.Context(), body, actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"policy": pol})
	  })
	   g.PUT("/policies/:id", func(c *gin.Context) {
		   var body PoolGuardPolicy
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  actor := actorFromCtx(c)
		  pol, err := svc.policyMgr.Update(c.Request.Context(), c.Param("id"), body, actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"policy": pol})
	  })
	   g.POST("/policies/:id/deactivate", func(c *gin.Context) {
		  actor := actorFromCtx(c)
		  if err := svc.policyMgr.Deactivate(c.Request.Context(), c.Param("id"), actor); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	   // ─── 防护单 ─────────────────────────────────────────────────────────
	   g.GET("/tickets", func(c *gin.Context) {
		  chain := c.Query("chain")
		   addr := c.Query("address")
		   state := TicketState(c.Query("state"))
		   tickets, err := svc.ticketMgr.ListTickets(c.Request.Context(), chain, addr, state)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"tickets": tickets, "count": len(tickets)})
	  })
	  g.POST("/tickets/:id/release", func(c *gin.Context) {
		  actor := actorFromCtx(c)
		  t, err := svc.ticketMgr.ReleaseTicket(c.Request.Context(), c.Param("id"), actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"ticket": t})
	   })
	  // ─── 审计 ───────────────────────────────────────────────────────────
	  g.GET("/audit", func(c *gin.Context) {
		  kind := AuditKind(c.Query("kind"))
		   refID := c.Query("ref_id")
		   entries, err := svc.store.ListAudit(c.Request.Context(), kind, refID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"entries": entries, "count": len(entries)})
	   })
  }

// requireOpsAdmin 中间件：要求 ops.admin 权限。

  func requireOpsAdmin(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.AbortWithStatusJSON(403, gin.H{"error": err.Error()})
		  return
	  }
	   c.Next()
   }

// actorFromCtx 从请求头或 callerid 提取操作人 ID（运维标识）。
   func actorFromCtx(c *gin.Context) string {
	   if v := c.GetHeader("X-Ops-Actor"); v != "" {
		   return strings.TrimSpace(v)
	   }
	   if v := c.GetHeader("X-User-Id"); v != "" {
		  return strings.TrimSpace(v)
	  }
	   return "unknown"
  }
