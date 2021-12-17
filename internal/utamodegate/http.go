// 功能：http.go
// utamodegate HTTP API——模式世代查询、租约管理、漂移双人确认、证明导出与 ops 接口。
// 路由（挂在 /v1 下）：
// GET /utamodegate/generation?user_id= — 查询用户当前模式世代
// GET /utamodegate/token?user_id= — 颁发世代令牌（借出/抵押调用前获取）
// POST /utamodegate/token/validate — 验证世代令牌（借出/抵押落账前检查）
// POST /utamodegate/mode/switch — 门控式模式切换（SetMode）
// GET /utamodegate/leases?user_id= — 查询用户活跃租约列表
// GET /utamodegate/lease/:id — 查询单条租约
// POST /utamodegate/bind/register — 注册在途负债/抵押绑定（借出/抵押模块调用）
// POST /utamodegate/bind/release — 释放绑定（还款/抵押释放时调用）
// GET /utamodegate/bind/:id — 查询单条绑定
// GET /utamodegate/drift-cases?user_id= — 查询用户 open/proven 漂移案
// GET /utamodegate/drift-case/:id — 查询单条漂移案
// POST /utamodegate/drift-case/open — 手动开立漂移案（ops）
// POST /utamodegate/drift-case/confirm — 四眼确认收口
// POST /utamodegate/proof/build — 构建并验证漂移证明（ops 手动触发）
// GET /utamodegate/proof/:id — 查询漂移证明
// GET /utamodegate/proof/:id/export — 导出证明摘要（核验存档）
// GET /utamodegate/stats — 引擎统计快照
// 权限：
// 查询接口 → ActQueryRead
// 写操作 → ActLedgerWrite（bind register/release）；ActOpsAdmin（mode switch / drift / proof）
// CreateDate: 2020.11.07

// 作者：jakson
// 日期：2021.12.16

package utamodegate

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 将 utamodegate 路由挂载到 /v1 路由组。

   func Register(v1 *gin.RouterGroup, _ *app.Runtime) {
	  svc := NewService(nil)
	   registerRoutes(v1, svc)
  }

// registerRoutes 内部用于注入 svc。
   func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	   g := v1.Group("/utamodegate")
	  // ── 世代查询 ──────────────────────────────────────────────────────────────
	   g.GET("/generation", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   userID := strings.TrimSpace(c.Query("user_id"))
		  if userID == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			   return
		  }
		  gen,
			  err := svc.GetGeneration(c.Request.Context(), userID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gen)
	   })
	   // ── 世代令牌颁发 ──────────────────────────────────────────────────────────
	   g.GET("/token", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  userID := strings.TrimSpace(c.Query("user_id"))
		  if userID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			  return
		  }
		   token,
			  err := svc.IssueToken(c.Request.Context(), userID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, token)
	  })
	   // ── 世代令牌验证 ──────────────────────────────────────────────────────────
	   g.POST("/token/validate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req struct {
			  UserID     string `json:"user_id"`
			   Generation int64  `json:"generation"`
		   }
		  if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			  return
		  }
		  if err := svc.ValidateToken(c.Request.Context(), req.UserID, req.Generation); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"valid": true})
	  })
	   // ── 模式切换 ──────────────────────────────────────────────────────────────
	  g.POST("/mode/switch", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req struct {
			  UserID string `json:"user_id"`
			  ToMode string `json:"to_mode"`
			   TriggerKind string `json:"trigger_kind"`
			  TriggerRef string `json:"trigger_ref"`
		  }
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			   return
		  }
		   result, err := svc.SetMode(c.Request.Context(), req.UserID, req.ToMode, req.TriggerKind, req.TriggerRef)
		  if err == ErrInflightLiability {
			   c.JSON(http.StatusConflict, gin.H{
				  "status": "pending",
				  "lease":  result.Lease,
				   "error":  err.Error(),
			  })
			  return
		  }
		   if err == ErrDriftBlocked {
			   c.JSON(http.StatusConflict, gin.H{
				   "status": "blocked",
				   "drift_case_id": result.DriftCaseID,
				   "error": err.Error(),
			  })
			   return
		  }
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, result)
	  })
	  // ── 租约查询 ──────────────────────────────────────────────────────────────
	   g.GET("/leases", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  userID := strings.TrimSpace(c.Query("user_id"))
		  if userID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			   return
		   }
		  leases, err := svc.ListActiveLeasesForUser(c.Request.Context(), userID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"leases": leases})
	  })
	  g.GET("/lease/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   lease, err := svc.GetLease(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, lease)
	   })
	  // ── 在途绑定管理 ──────────────────────────────────────────────────────────
	   g.POST("/bind/register", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   RegisterBindReq
			  Generation int64 `json:"generation"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			   return
		   }
		  bind, err := svc.RegisterBind(c.Request.Context(), body.RegisterBindReq, body.Generation)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusCreated, bind)
	   })
	   g.POST("/bind/release", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req ReleaseBindReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			  return
		   }
		   bind,
			  err := svc.ReleaseBind(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, bind)
	  })
	   g.GET("/bind/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  bind, err := svc.GetBind(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, bind)
	  })
	  // ── 漂移案管理 ────────────────────────────────────────────────────────────
	   g.GET("/drift-cases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  userID := strings.TrimSpace(c.Query("user_id"))
		   if userID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			  return
		  }
		  cases, err := svc.ListBlockingDriftCases(c.Request.Context(), userID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"drift_cases": cases})
	   })
	  g.GET("/drift-case/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  dc, err := svc.GetDriftCase(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, dc)
	   })
	  g.POST("/drift-case/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req OpenDriftCaseReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			  return
		   }
		   dc, err := svc.OpenDriftCase(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusCreated, dc)
	   })
	   g.POST("/drift-case/confirm", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req ConfirmCloseReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			   return
		   }
		  dc, err := svc.ConfirmClose(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, dc)
	   })
	  // ── 漂移证明 ──────────────────────────────────────────────────────────────
	  g.POST("/proof/build", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req BuildProofReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			   return
		  }
		   proof, dc, err := svc.BuildAndAttachProof(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusCreated, gin.H{"proof": proof, "drift_case": dc})
	   })
	   g.GET("/proof/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  proof, err := svc.GetProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, proof)
	  })
	  g.GET("/proof/:id/export", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  export, err := svc.ExportProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, export)
	  })
	   // ── 引擎统计 ──────────────────────────────────────────────────────────────
	   g.GET("/stats", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"stats": svc.Stats()})
	   })
   }
