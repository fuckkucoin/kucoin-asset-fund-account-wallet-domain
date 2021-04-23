package destentrygate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 挂载 HTTP 路由。

  func Register(v1 *gin.RouterGroup, _ *app.Runtime, svc *Service) {
	  if v1 == nil || svc == nil {
		  return
	   }
	   g := v1.Group("/destentrygate")
	   g.POST("/precheck", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			  Chain string `json:"chain"`
			   Address    string `json:"address"`
			  TTLSeconds int64  `json:"ttl_seconds"`
			  IdempotencyKey string `json:"idempotency_key"`
			  Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   lease, err := svc.PrecheckWithLease(c.Request.Context(), PrecheckWithLeaseReq{
			   UserID: body.UserID, Chain: body.Chain, Address: body.Address,
			  TTLSeconds: body.TTLSeconds, IdempotencyKey: body.IdempotencyKey, Actor: body.Actor,
		   })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, lease)
	  })
	   g.POST("/assert", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  LeaseID string `json:"dest_entry_lease_id"`
			   UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
			   Address        string `json:"address"`
			   IdempotencyKey string `json:"idempotency_key"`
			   Consume bool `json:"consume"`
			  Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   res, err := svc.AssertLease(c.Request.Context(), AssertLeaseReq{
			   LeaseID: body.LeaseID,
			   UserID: body.UserID, Chain: body.Chain, Address: body.Address,
			   IdempotencyKey: body.IdempotencyKey, Consume: body.Consume, Actor: body.Actor,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, res)
	   })
	   g.POST("/renew", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  LeaseID string `json:"dest_entry_lease_id"`
			  TTLSeconds int64 `json:"ttl_seconds"`
			   IdempotencyKey string `json:"idempotency_key"`
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   lease, err := svc.RenewLease(c.Request.Context(), RenewLeaseReq{
			  LeaseID: body.LeaseID, TTLSeconds: body.TTLSeconds,
			  IdempotencyKey: body.IdempotencyKey,
			   Actor:          body.Actor,
		   })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, lease)
	  })
	  g.POST("/revoke", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  LeaseID string `json:"dest_entry_lease_id"`
			  Actor   string `json:"actor"`
			   Reason string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  lease,
			  err := svc.OpsRevoke(c.Request.Context(), body.LeaseID, body.Actor, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, lease)
	   })
	   g.POST("/lifecycle", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Kind string `json:"kind"`
			  UserID  string `json:"user_id"`
			   Chain   string `json:"chain"`
			  Address string `json:"address"`
			   EntryID string `json:"entry_id"`
			  Actor   string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   live := LiveEntry{
			  EntryID: body.EntryID, UserID: body.UserID, Chain: body.Chain, Address: body.Address,
		  }
		  if live.EntryID == "" {
			   live.EntryID = DeriveEntryID(live.UserID, live.Chain, live.Address)
		   }
		  var (
			  n int
			  err error
		   )
		  switch body.Kind {
		   case "temp_expire":
			   n, err = svc.OnTempExpired(c.Request.Context(), live, body.Actor)
		  case "breakglass_reclaim":
			  n, err = svc.OnBreakglassReclaim(c.Request.Context(), live, body.Actor)
		   case "cooldown_not_full":
			  n, err = svc.OnRestoreCooldownNotFull(c.Request.Context(), live, body.Actor)
		  default:
			   n, err = svc.OnEntryRevoke(c.Request.Context(), live, body.Actor)
		   }
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"revoked": n, "entry_id": live.EntryID, "kind": body.Kind})
	   })
	   g.GET("/lease/:id", func(c *gin.Context) {
		  lease, err := svc.GetLease(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, lease)
	  })
	  g.GET("/proof/:id", func(c *gin.Context) {
		  p, err := svc.GetProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, p)
	   })
	  g.POST("/proofs/export", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  rows, err := svc.ExportProofs(c.Request.Context())
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"items": rows, "count": len(rows)})
	   })
	   g.GET("/audit/:id", func(c *gin.Context) {
		   rows, err := svc.ListAudit(c.Request.Context(), c.Param("id"), 100)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"items": rows})
	  })
	  g.GET("/proofs", func(c *gin.Context) {
		   rows, err := svc.ListProofs(c.Request.Context(), ExportProofFilter{
			  Kind: ProofKind(c.Query("kind")),
			  EntryID: c.Query("entry_id"),
			   UserID: c.Query("user_id"),
			  Limit:  100,
		   })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"items": rows})
	  })
   }
