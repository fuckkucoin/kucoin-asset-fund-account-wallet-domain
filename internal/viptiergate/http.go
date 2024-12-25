package viptiergate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 将 viptiergate 路由挂载到 /v1 路由组。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := NewService(nil)
	  registerRoutes(v1, svc)
   }

// registerRoutes 内部用于注入 svc。
  func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	   // ── 世代查询 ──────────────────────────────────────────────────────────────
	  v1.GET("/viptiergate/generation", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   userID := c.Query("user_id")
		  if userID == "" {
			  c.JSON(400, gin.H{"error": "user_id required"})
			  return
		   }
		   gen, err := svc.GetGeneration(c.Request.Context(), userID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gen)
	   })
	   // ── 租约查询 ──────────────────────────────────────────────────────────────
	  v1.GET("/viptiergate/leases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   userID := c.Query("user_id")
		   if userID == "" {
			  c.JSON(400, gin.H{"error": "user_id required"})
			   return
		  }
		   leases, err := svc.ListActiveLeasesForUser(c.Request.Context(), userID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"leases": leases})
	   })
	   v1.GET("/viptiergate/lease/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  lease, err := svc.GetLease(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, lease)
	  })
	  // ── QuoteBind 查询与结算 ──────────────────────────────────────────────────
	  v1.GET("/viptiergate/quote-binds/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   qb, err := svc.GetQuoteBind(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, qb)
	   })
	  v1.POST("/viptiergate/quote-settle", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body SettleQuoteReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   qb, err := svc.SettleQuote(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, qb)
	   })
	   // ── 在途冻额管理 ──────────────────────────────────────────────────────────
	  v1.POST("/viptiergate/freeze/register", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			   Amount string `json:"amount"`
			   BPS    int    `json:"bps"`
			   Tier   Tier   `json:"tier"`
			  Generation int64 `json:"generation"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  f, err := svc.RegisterFreeze(c.Request.Context(),
			   body.UserID, body.Asset, body.Amount, body.BPS, body.Tier, body.Generation)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, f)
	   })
	  v1.POST("/viptiergate/freeze/release", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   FreezeID string `json:"freeze_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := svc.ReleaseFreeze(c.Request.Context(), body.FreezeID); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"released": true})
	   })
	  // ── 漂移案管理 ────────────────────────────────────────────────────────────
	   v1.GET("/viptiergate/drift-cases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  userID := c.Query("user_id")
		  if userID == "" {
			  c.JSON(400, gin.H{"error": "user_id required"})
			  return
		  }
		   cases, err := svc.ListOpenDriftCasesForUser(c.Request.Context(), userID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"drift_cases": cases})
	   })
	  v1.GET("/viptiergate/drift-case/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  dc, err := svc.GetDriftCase(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, dc)
	   })
	   v1.POST("/viptiergate/drift-case/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body OpenDriftCaseReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   dc, err := svc.OpenDriftCase(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, dc)
	  })
	  v1.POST("/viptiergate/drift-case/confirm", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body ConfirmDriftReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  dc, err := svc.ConfirmDriftClose(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, dc)
	  })
	  // ── 漂移证明管理 ──────────────────────────────────────────────────────────
	   v1.GET("/viptiergate/drift-proof/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  p, err := svc.GetDriftProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, p)
	  })
	  v1.POST("/viptiergate/drift-proof/build", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   CaseID string `json:"drift_case_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   p, err := svc.BuildAndVerifyProof(c.Request.Context(), body.CaseID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, p)
	   })
	   v1.GET("/viptiergate/drift-proof/:id/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  p, err := svc.ExportProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, p)
	   })
	   // ── 引擎统计 ──────────────────────────────────────────────────────────────
	   v1.GET("/viptiergate/stats", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, svc.Stats())
	  })
  }
