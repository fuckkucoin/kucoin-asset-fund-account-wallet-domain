package legmovefence

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 挂载 HTTP / ops API。

   func Register(v1 *gin.RouterGroup, _ *app.Runtime, svc *Service) {
	  if v1 == nil || svc == nil {
		   return
	   }
	  g := v1.Group("/legmovefence")
	   g.POST("/exec", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   PlanID string `json:"plan_id"`
			   LegID string `json:"leg_id"`
			   IdempotencyKey string `json:"idempotency_key"`
			   TTLSeconds     int64  `json:"ttl_seconds"`
			   Actor         string `json:"actor"`
			   ForceTransfer bool   `json:"force_transfer"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   res, err := svc.ExecLegWithFence(c.Request.Context(), ExecLegReq{
			  PlanID: body.PlanID,
			   LegID: body.LegID, IdempotencyKey: body.IdempotencyKey,
			  TTLSeconds: body.TTLSeconds, Actor: body.Actor, ForceTransfer: body.ForceTransfer,
		   })
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, res)
	   })
	   g.POST("/retry", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   FenceID string `json:"leg_move_fence_id"`
			  IdempotencyKey string `json:"idempotency_key"`
			  Actor          string `json:"actor"`
			   ForceTransfer  bool   `json:"force_transfer"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   res, err := svc.RetryAssertFence(c.Request.Context(), RetryReq{
			  FenceID: body.FenceID, IdempotencyKey: body.IdempotencyKey,
			   Actor: body.Actor, ForceTransfer: body.ForceTransfer,
		  })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, res)
	  })
	  g.POST("/resume", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  CaseID string `json:"case_id"`
			   Actor string `json:"actor"`
			  MarkOnly bool `json:"mark_only"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   cs, err := svc.OpsResume(c.Request.Context(), ResumeReq{CaseID: body.CaseID, Actor: body.Actor, MarkOnly: body.MarkOnly})
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, cs)
	  })
	  g.POST("/halt-revoke", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   RegionID string `json:"region_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  n,
			   err := svc.RevokeHalted(c.Request.Context(), body.RegionID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"revoked": n})
	   })
	  g.GET("/fence/:id", func(c *gin.Context) {
		  f, err := svc.GetFence(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, f)
	  })
	   g.GET("/proof/:id", func(c *gin.Context) {
		   p, err := svc.GetProof(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, ProofViewOf(p))
	   })
	   g.GET("/proofs", func(c *gin.Context) {
		   list, err := svc.ListProofs(c.Request.Context(), ExportFilter{
			   Kind: ProofKind(c.Query("kind")), FenceID: c.Query("fence_id"), LegID: c.Query("leg_id"), Limit: 100,
		  })
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"items": list})
	  })
	  g.POST("/proofs/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   n, err := svc.ExportProofs(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"exported": n})
	  })
	  g.GET("/audit/:id", func(c *gin.Context) {
		  list, err := svc.ListAudit(c.Request.Context(), c.Param("id"), 100)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"items": list})
	  })
	   g.GET("/half/:id", func(c *gin.Context) {
		  h, err := svc.GetHalf(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, h)
	   })
	   g.POST("/patrol", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   n,
			  err := svc.PatrolHalf(c.Request.Context())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"resumed": n})
	   })
  }
