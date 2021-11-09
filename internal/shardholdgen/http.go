package shardholdgen

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/wdshard"
	"github.com/gin-gonic/gin"
)

// Register 挂载 HTTP / ops API。
  func Register(v1 *gin.RouterGroup, _ *app.Runtime, svc *Service) {
	  if v1 == nil || svc == nil {
		   return
	  }
	  g := v1.Group("/shardholdgen")
	  g.POST("/advance", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   PlanID         string `json:"plan_id"`
			  LegID          string `json:"leg_id"`
			  IdempotencyKey string `json:"idempotency_key"`
			   TTLSeconds    int64  `json:"ttl_seconds"`
			   Actor         string `json:"actor"`
			   SkipBroadcast bool   `json:"skip_broadcast"`
			   ForceFail     bool   `json:"force_fail"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   res, err := svc.AdvanceWithGen(c.Request.Context(), AdvanceReq{
			  PlanID: body.PlanID, LegID: body.LegID, IdempotencyKey: body.IdempotencyKey,
			   TTLSeconds: body.TTLSeconds, Actor: body.Actor,
			   SkipBroadcast: body.SkipBroadcast, ForceFail: body.ForceFail,
		   })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, res)
	   })
	   g.POST("/release", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   PlanID         string `json:"plan_id"`
			  LegID          string `json:"leg_id"`
			  GenID          string `json:"leg_hold_gen_id"`
			   Amount         int64  `json:"amount"`
			   IdempotencyKey string `json:"idempotency_key"`
			  Actor          string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  res, err := svc.ReleaseAssert(c.Request.Context(), ReleaseReq{
			   PlanID: body.PlanID,
			   LegID:  body.LegID, GenID: body.GenID,
			  Amount: body.Amount, IdempotencyKey: body.IdempotencyKey, Actor: body.Actor,
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
			  Actor  string `json:"actor"`
			   MarkOnly bool `json:"mark_only"`
			  Abandon  bool `json:"abandon"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  res, err := svc.ResumeHalf(c.Request.Context(), ResumeReq{
			  CaseID: body.CaseID,
			  Actor: body.Actor, MarkOnly: body.MarkOnly, Abandon: body.Abandon,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, res)
	  })
	   g.POST("/align", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  PlanID    string `json:"plan_id"`
			   LegID     string `json:"leg_id"`
			  FillDelta int64  `json:"fill_delta"`
			   TxHash    string `json:"tx_hash"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   leg, err := svc.AlignWithGen(c.Request.Context(), wdshard.AlignHint{
			  PlanID: body.PlanID, LegID: body.LegID, FillDelta: body.FillDelta, TxHash: body.TxHash,
		   })
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, leg)
	  })
	   g.GET("/gen/:id", func(c *gin.Context) {
		  g, err := svc.GetGen(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, g)
	  })
	  g.GET("/proof/:id", func(c *gin.Context) {
		   p, err := svc.GetProof(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, p)
	   })
	  g.GET("/proofs", func(c *gin.Context) {
		   list, err := svc.ListProofs(c.Request.Context(), ExportFilter{
			  LegID: c.Query("leg_id"), Limit: 100, UnexportedOnly: c.Query("unexported") == "1",
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
		   list, err := svc.ExportProofs(c.Request.Context(), ExportFilter{UnexportedOnly: true, Limit: ProofExportBatch})
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"items": list, "count": len(list)})
	   })
	   g.GET("/audit/:gen_id", func(c *gin.Context) {
		   list, err := svc.ListAudit(c.Request.Context(), c.Param("gen_id"), 100)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"items": list})
	  })
   }
