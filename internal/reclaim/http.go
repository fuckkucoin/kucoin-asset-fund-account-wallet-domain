// @FileName: http.go
// @Summary: 错链/未认领充值回收 HTTP API——运营台/Desk 调用入口。
// 路由前缀：/v1/reclaim/*
// 端点列表：
// POST /reclaim/open 开立回收案件
// POST /reclaim/reserve 预占链上输出
// POST /reclaim/reclaim 完成回收操作
// POST /reclaim/cancel 取消回收
// POST /reclaim/escalate 人工升级
// POST /reclaim/auto-credit 同链自动贷记成功通知
// POST /reclaim/sweep-legs 创建 Sweep leg 目录
// POST /reclaim/sla-tick 手动触发 SLA 检查（ops/admin）
// GET /reclaim/case 获取单个案件
// GET /reclaim/cases 列举案件（带过滤）
// GET /reclaim/timeline 获取案件时间轴
// GET /reclaim/sweep-legs 获取案件 Sweep leg 目录
// GET /reclaim/occupancy 查看当前占用快照（管理员）
// GET /reclaim/race-events 消费竞态事件（管理员）
// 权限：
// 写操作需 ActAMLOpen 或 ActOpsAdmin。
// 读操作需 ActCaseRead。
// occupancy/race-events/sla-tick 端点需 ActOpsAdmin。
// @Author: Draven
// @Created: 2021.10.13

package reclaim

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Register 注册 reclaim HTTP 路由到 v1 路由组。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  _ = rt
	   occ := NewReclaimOccupancy()
	  store := NewStore(persist.NewMemKV())
	   svc := NewService(occ, store)
	  g := v1.Group("/reclaim")
	   // ─── 写操作 ────────────────────────────────────────────────────────────
	  g.POST("/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body OpenReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.OpenedBy == "" {
			  body.OpenedBy = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got, err := svc.Open(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  g.POST("/reserve", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body ReserveReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got, err := svc.Reserve(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  g.POST("/reclaim", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body ReclaimReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   got, err := svc.Reclaim(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	  g.POST("/cancel", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body CancelReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   got,
			   err := svc.Cancel(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	  g.POST("/escalate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body EscalateReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   got, err := svc.Escalate(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	   g.POST("/auto-credit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body AutoCreditReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Actor == "" {
			   body.Actor = "system"
		  }
		  got, err := svc.AutoCredit(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	   g.POST("/sweep-legs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body CreateLegsReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   group, err := svc.CreateSweepLegs(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   recordSweepLegsCreated(group.Chain, group.Asset)
		   c.JSON(200, group)
	   })
	   g.POST("/sla-tick", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   n := svc.TickSLA(c.Request.Context())
		   c.JSON(200, gin.H{"escalated": n})
	   })
	   // ─── 读操作 ────────────────────────────────────────────────────────────
	   g.GET("/case", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   id := strings.TrimSpace(c.Query("id"))
		   if id == "" {
			  c.JSON(400, gin.H{"error": "id required"})
			  return
		  }
		  got, err := svc.GetCase(id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  g.GET("/cases", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   req := ListReq{
			   Chain: c.Query("chain"),
			  Asset: c.Query("asset"),
			  State: c.Query("state"),
			  UserID: c.Query("user_id"),
			  Kind:   c.Query("kind"),
			   Limit:  parseIntQuery(c, "limit", 50),
			  Offset: parseIntQuery(c, "offset", 0),
		  }
		  c.JSON(200, svc.ListCases(req))
	  })
	   g.GET("/timeline", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  id := strings.TrimSpace(c.Query("id"))
		  if id == "" {
			   c.JSON(400, gin.H{"error": "id required"})
			  return
		  }
		  c.JSON(200, svc.GetTimeline(id))
	  })
	  g.GET("/sweep-legs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  id := strings.TrimSpace(c.Query("id"))
		   if id == "" {
			   c.JSON(400, gin.H{"error": "id required"})
			  return
		  }
		  group := svc.GetSweepLegs(id)
		   if group == nil {
			  c.JSON(404, gin.H{"error": "sweep legs not found"})
			  return
		  }
		  c.JSON(200, group)
	  })
	  g.GET("/occupancy", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{
			   "held":  svc.Occupancy().Snapshot(),
			   "count": svc.Occupancy().Len(),
		  })
	   })
	   g.GET("/race-events", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  evs := svc.LateCreditMutex().DrainRaceEvents()
		   c.JSON(200, gin.H{
			  "events": evs,
			   "count":   len(evs),
			   "dropped": svc.LateCreditMutex().RaceDropCount(),
		   })
	  })
   }

// parseIntQuery 从 query 参数解析整数，失败时返回默认值。
   func parseIntQuery(c *gin.Context, key string, def int) int {
	  s := c.Query(key)
	   if s == "" {
		  return def
	  }
	  n := 0
	   for _, ch := range s {
		  if ch < '0' || ch > '9' {
			   return def
		   }
		   n = n*10 + int(ch-'0')
	  }
	   return n
   }
