package wdshard

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 注册 /wdshard 路由组。

   func RegisterRoutes(v1 *gin.RouterGroup, svc *Service) {
	  g := v1.Group("/wdshard")
	   // 计划创建与预览。
	   g.POST("/preview", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req PreviewReq
		  if !mw.BindJSON(c, &req) {
			  return
		  }
		   resp, err := svc.Preview(req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	  })
	  g.POST("/plans", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req CreatePlanReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		   plan,
			   legs, err := svc.Create(req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"plan": plan, "legs": legs})
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
		   legs, _ := svc.GetLegs(plan.ID)
		   c.JSON(http.StatusOK, gin.H{"plan": plan, "legs": legs})
	  })
	   g.GET("/plans/:id/progress", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  prog, err := svc.Progress(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, prog)
	  })
	   g.GET("/plans", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   userID := c.Query("user_id")
		  plans := svc.ListPlans(userID, 50)
		   c.JSON(http.StatusOK, gin.H{"plans": plans})
	  })
	   // 运维操作：批准、启动、暂停、续跑、中止。
	   g.POST("/plans/:id/approve", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  plan, err := svc.Approve(c.Param("id"), body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, plan)
	   })
	   g.POST("/plans/:id/start", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  plan, err := svc.Start(c.Param("id"), body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, plan)
	   })
	  g.POST("/plans/pause", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var req PauseReq
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		   plan, err := svc.Pause(req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, plan)
	  })
	   g.POST("/plans/resume", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var req ResumeReq
		  if !mw.BindJSON(c, &req) {
			   return
		  }
		  plan, err := svc.Resume(req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, plan)
	  })
	   g.POST("/plans/:id/abort", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Reason string `json:"reason"`
			  Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   plan, err := svc.Abort(c.Param("id"), body.Actor, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, plan)
	  })
	  // 内部：腿状态推进（供 workers 或 CI 使用）。
	   g.POST("/legs/advance", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var req LegAdvanceReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		   leg, plan, err := svc.AdvanceLeg(req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"leg": leg, "plan": plan})
	   })
	   // 策略管理。
	  g.GET("/policies", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"policies": svc.ListPolicies()})
	  })
	   g.POST("/policies", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var pol PlanPolicy
		  if !mw.BindJSON(c, &pol) {
			  return
		   }
		   if err := svc.AddPolicy(pol); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"policy": pol})
	  })
	   // 用户聚合视图。
	   g.GET("/aggregate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  userID := c.Query("user_id")
		   asset := c.Query("asset")
		   if userID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			   return
		   }
		  filled, inflight, remain, err := svc.UserAggregate(userID, asset)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{
			  "user_id": userID,
			   "asset":   asset,
			  "filled":  filled.String(),
			  "inflight": inflight.String(),
			   "remain":   remain.String(),
		  })
	  })
	   // 停机开关。
	   g.POST("/halt", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   On bool `json:"on"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  svc.SetHalt(body.On)
		  c.JSON(http.StatusOK, gin.H{"halt": svc.Halted()})
	   })
   }
