package fiatbreak

import (
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fiatrail"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Register 挂载 fiatbreak 路由到 v1 路由组。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   var rail *fiatrail.Service
	  if rt != nil {
		  // rail 服务由外部注入；此处兼容 nil（内存模式）
		  _ = rt
	  }
	  rail = fiatrail.New()
	  svc := NewBreakService(rail)
	  aging := NewAgingService(svc, svc.Suspense())
	  worker := NewWorker(svc, svc.Suspense(), aging, DefaultWorkerConfig())
	   _ = worker // 由 cmd/fiat-break 启动；此处仅初始化
	  g := v1.Group("/fiatbreak")
	  // ──────────────────────────────────────────
	   // 断裂检测
	   // ──────────────────────────────────────────
	  g.POST("/detect", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var req DetectReq
		   if !mw.BindJSON(c, &req) {
			   return
		  }
		  result, err := svc.Detect(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, result)
	  })
	  // ──────────────────────────────────────────
	   // 断裂 CRUD
	   // ──────────────────────────────────────────
	  g.GET("/breaks", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   filterState := BreakState(strings.TrimSpace(c.Query("state")))
		  filterKind := BreakKind(strings.TrimSpace(c.Query("kind")))
		  limit := validate.Limit(0, 50, 200)
		  c.JSON(200, gin.H{
			   "breaks": svc.List(filterState, filterKind, limit),
		  })
	  })
	  g.GET("/breaks/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   br, err := svc.Get(c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, br)
	   })
	  g.POST("/breaks/:id/investigate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   AssignedTo string `json:"assigned_to"`
			   Actor      string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   br, err := svc.Investigate(c.Param("id"), body.AssignedTo, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, br)
	   })
	   g.POST("/breaks/:id/resolve", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Actor string `json:"actor"`
			  Note string `json:"note"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  br, err := svc.Resolve(c.Request.Context(), c.Param("id"), body.Actor, body.Note)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, br)
	  })
	  g.POST("/breaks/:id/write_off", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Actor string `json:"actor"`
			   Note  string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   br, err := svc.WriteOff(c.Request.Context(), c.Param("id"), body.Actor, body.Note)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, br)
	   })
	   // ──────────────────────────────────────────
	   // 挂账
	  // ──────────────────────────────────────────
	  g.GET("/suspense", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   filterState := SuspenseState(strings.TrimSpace(c.Query("state")))
		  limit := validate.Limit(0, 50, 200)
		  c.JSON(200, gin.H{
			   "suspense": svc.Suspense().List(filterState, limit),
			   "totals": svc.Suspense().TotalOpen(),
		   })
	  })
	   g.GET("/suspense/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   e, err := svc.Suspense().Get(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, e)
	   })
	  g.POST("/suspense/dispose", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var req SuspenseDisposeReq
		  if !mw.BindJSON(c, &req) {
			  return
		   }
		   var (
			  e   SuspenseEntry
			  err error
		  )
		  switch strings.ToLower(strings.TrimSpace(req.Action)) {
		   case "release":
			  e,
				  err = svc.Suspense().Release(c.Request.Context(), req.SuspenseID, req.Actor, req.Note)
		   case "consume":
			  e, err = svc.Suspense().Consume(c.Request.Context(), req.SuspenseID, req.Actor, req.Note)
		   default:
			   c.JSON(400, gin.H{"error": "action must be release or consume"})
			   return
		  }
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, e)
	   })
	  // ──────────────────────────────────────────
	  // 双边证明
	   // ──────────────────────────────────────────
	  g.GET("/proofs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  req := ProofQueryReq{
			   BookingID: strings.TrimSpace(c.Query("booking_id")),
			  BreakID:   strings.TrimSpace(c.Query("break_id")),
			   Limit: validate.Limit(0,
				  50, 200),
		  }
		  c.JSON(200, gin.H{"proofs": svc.Proof().Query(req)})
	   })
	  g.POST("/proofs/verify", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  ID string `json:"id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  ok, err := svc.Proof().Verify(body.ID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"id": body.ID, "valid": ok})
	  })
	   // ──────────────────────────────────────────
	  // 双控强制冲销
	   // ──────────────────────────────────────────
	   g.POST("/force_reverse", func(c *gin.Context) {
		   // 强制冲销需要双重权限校验
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var req ForceReverseReq
		  if !mw.BindJSON(c, &req) {
			  return
		  }
		   result, err := handleForceReverse(c.Request.Context(), svc, req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, result)
	   })
	  // ──────────────────────────────────────────
	   // SLA 策略与老化事件
	   // ──────────────────────────────────────────
	   g.GET("/policies", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"policies": svc.Policies()})
	   })
	   g.GET("/aging/events", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  limit := validate.Limit(0, 100, 500)
		   c.JSON(200, gin.H{"events": aging.Events(limit)})
	   })
  }
