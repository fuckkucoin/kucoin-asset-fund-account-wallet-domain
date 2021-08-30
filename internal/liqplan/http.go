package liqplan

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 注册 liqplan 路由到 v1 路由组。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   _ = rt
	  g := v1.Group("/liqplan")
	   // ── 查询 ───────────────────────────────────────────────────────────
	   g.GET("/plans", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   state := c.Query("state")
		   userID := c.Query("user_id")
		  var plans []LiqPlan
		   if userID != "" {
			  plans = svc.ListByUser(userID)
		   } else {
			   plans = svc.ListPlans(state)
		   }
		   c.JSON(http.StatusOK, gin.H{"plans": plans, "host": "account-book.kucoin.net"})
	   })
	  g.GET("/plans/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   plan, err := svc.GetPlan(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, plan)
	   })
	  g.GET("/plans/:id/legs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   legs := svc.ListLegs(c.Param("id"))
		   c.JSON(http.StatusOK, gin.H{"legs": legs, "host": "account-book.kucoin.net"})
	  })
	   g.GET("/plans/:id/proof", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   proof, err := svc.GetProofByPlan(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, proof)
	  })
	   // ── 触发强平 ──────────────────────────────────────────────────────
	  g.POST("/trigger", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   BorrowID       string `json:"borrow_id"`
			  IdempotencyKey string `json:"idempotency_key"`
			  UserID      string `json:"user_id"`
			  BorrowAsset string `json:"borrow_asset"`
			  BorrowAmount    string `json:"borrow_amount"`
			   CollateralID    string `json:"collateral_id"`
			   CollateralAsset string `json:"collateral_asset"`
			  CollateralAmount string `json:"collateral_amount"`
			   TriggerLTVBPS    int    `json:"trigger_ltv_bps"`
			  CapBPS           int    `json:"cap_bps"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  plan, err := svc.TriggerPlan(TriggerReq{
			   BorrowID: body.BorrowID,
			  IdempotencyKey: body.IdempotencyKey,
			   UserID:         body.UserID,
			   BorrowAsset: body.BorrowAsset,
			   BorrowAmount: body.BorrowAmount,
			   CollateralID: body.CollateralID,
			   CollateralAsset:  body.CollateralAsset,
			   CollateralAmount: body.CollateralAmount,
			   TriggerLTVBPS: body.TriggerLTVBPS,
			  CapBPS:        body.CapBPS,
		   })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, plan)
	   })
	   // ── 中止计划（双人） ──────────────────────────────────────────────
	   g.POST("/plans/:id/abort", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Actor1 string `json:"actor1"`
			   Actor2 string `json:"actor2"`
			  Reason string `json:"reason"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  plan, err := svc.AbortPlan(AbortReq{
			  PlanID: c.Param("id"), Actor1: body.Actor1, Actor2: body.Actor2, Reason: body.Reason,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, plan)
	  })
	  // ── 续跑计划腿（双人）───────────────────────────────────────────
	  g.POST("/plans/:id/resume", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  LegID string `json:"leg_id"`
			  Actor1 string `json:"actor1"`
			   Actor2 string `json:"actor2"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.Actor1 == "" || body.Actor2 == "" || body.Actor1 == body.Actor2 {
			  c.JSON(http.StatusBadRequest, gin.H{"error": ErrAbortUnauthorized.Error()})
			   return
		  }
		   leg, err := svc.ResumeLeg(body.LegID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, leg)
	  })
	   // ── 回滚单腿 ──────────────────────────────────────────────────────
	   g.POST("/legs/:id/rollback", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  leg, _, err := svc.RollbackLeg(c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, leg)
	  })
	  // ── 挂账单腿 ──────────────────────────────────────────────────────
	   g.POST("/legs/:id/suspend", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Note string `json:"note"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  leg, err := svc.SuspendLeg(c.Param("id"), body.Note)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, leg)
	   })
	  // ── 构建证明 ──────────────────────────────────────────────────────
	   g.POST("/plans/:id/proof/build", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  proof, err := svc.BuildProof(c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, proof)
	   })
	   // ── 校验证明（双人） ─────────────────────────────────────────────
	  g.POST("/proof/:id/validate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Actor1 string `json:"actor1"`
			  Actor2 string `json:"actor2"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  proof, err := svc.ValidateProof(c.Param("id"), body.Actor1, body.Actor2)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, proof)
	  })
	  // ── 证明导出 ──────────────────────────────────────────────────────
	  g.GET("/proof/:id/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   proof, err := svc.GetProof(c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.Header("Content-Type", "application/json")
		   c.Header("Content-Disposition", "attachment; filename=liq-proof-"+proof.ID+".json")
		   c.JSON(http.StatusOK, proof)
	  })
	  // ── 标价世代管理 ─────────────────────────────────────────────────
	  g.POST("/mark/register", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body MarkGate
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   svc.RegisterMark(body)
		   c.JSON(http.StatusOK, gin.H{"ok": true})
	  })
	   g.POST("/mark/poison", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  MarkGateID string `json:"mark_gate_id"`
			  Note string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if err := svc.PoisonMark(body.MarkGateID, body.Note); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"ok": true})
	  })
	  // ── ops 扫描触发 ─────────────────────────────────────────────────
	   g.POST("/scan/ltv", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   // 手动触发 LTV 扫描（无业务 ltvReader 时仅返回空报告）
		   w := NewLTVScanWorker(svc, svc.ltvReader, nil)
		   rpt := w.RunOnce()
		  c.JSON(http.StatusOK, gin.H{"report": rpt, "host": "account-book.kucoin.net"})
	   })
   }
