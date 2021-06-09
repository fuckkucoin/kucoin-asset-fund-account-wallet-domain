package fundintgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 挂载 HTTP 路由。

  func Register(v1 *gin.RouterGroup, _ *app.Runtime, svc *Service) {
	  g := v1.Group("/fundintgate")
	   g.POST("/check", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			  ToUID   string `json:"to_uid"`
			  ToEmail string `json:"to_email"`
			   ToPhone string `json:"to_phone"`
			  Asset   string `json:"asset"`
			  Amount string `json:"amount"`
			  TTLSeconds     int64  `json:"ttl_seconds"`
			  IdempotencyKey string `json:"idempotency_key"`
			   Actor          string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   lease, err := svc.CheckWithLease(c.Request.Context(), CheckWithLeaseReq{
			  UserID: body.UserID, ToUID: body.ToUID, ToEmail: body.ToEmail, ToPhone: body.ToPhone,
			   Asset: body.Asset, Amount: body.Amount, TTLSeconds: body.TTLSeconds,
			   IdempotencyKey: body.IdempotencyKey, Actor: body.Actor,
		   })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, lease)
	  })
	  g.POST("/send", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   LeaseID string `json:"int_gate_lease_id"`
			  UserID  string `json:"user_id"`
			   ToUID   string `json:"to_uid"`
			  ToEmail string `json:"to_email"`
			   ToPhone string `json:"to_phone"`
			  Asset string `json:"asset"`
			  Amount         string `json:"amount"`
			   IdempotencyKey string `json:"idempotency_key"`
			   Remark         string `json:"remark"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   res, err := svc.SendAssert(c.Request.Context(), SendAssertReq{
			   LeaseID: body.LeaseID, UserID: body.UserID, ToUID: body.ToUID,
			  ToEmail: body.ToEmail, ToPhone: body.ToPhone, Asset: body.Asset,
			   Amount: body.Amount, IdempotencyKey: body.IdempotencyKey, Remark: body.Remark,
		  })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, res)
	   })
	  g.POST("/halt", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Actor string `json:"actor"`
			  Reason string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  gen, n, err := svc.HaltGate(c.Request.Context(), body.Actor, body.Reason)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	  })
	  g.POST("/resume", func(c *gin.Context) {
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
		   gen,
			   n, err := svc.ResumeGate(c.Request.Context(), body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	   })
	   g.POST("/swap-impl", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  ImplID string `json:"gate_impl_id"`
			   Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   gen, n, err := svc.SwapGateImpl(c.Request.Context(), body.ImplID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	  })
	   g.POST("/forbid", func(c *gin.Context) {
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
		   gen, n, err := svc.RaiseForbidWithdraw(c.Request.Context(), body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	   })
	   g.POST("/forbid/clear", func(c *gin.Context) {
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
		  gen, n, err := svc.ClearForbidWithdraw(c.Request.Context(), body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	  })
	   g.POST("/revoke", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   LeaseID string `json:"int_gate_lease_id"`
			   Reason  string `json:"reason"`
			   Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   lease, err := svc.OpsRevoke(c.Request.Context(), OpsRevokeReq{
			   LeaseID: body.LeaseID, Reason: body.Reason, Actor: body.Actor,
		   })
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, lease)
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
	   g.GET("/proof/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
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
		   var body struct {
			  Kind string `json:"kind"`
			  LeaseID string `json:"int_gate_lease_id"`
			   Limit int `json:"limit"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   list, err := svc.ExportProofs(c.Request.Context(), ExportProofFilter{
			   Kind: ProofKind(body.Kind), LeaseID: body.LeaseID, Limit: body.Limit,
		  })
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"proofs": list, "count": len(list)})
	  })
	   g.GET("/audit/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  list, err := svc.ListAudit(c.Request.Context(), c.Param("id"), 200)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"audit": list})
	   })
	   g.GET("/surface", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   surf, err := svc.LiveSurface(c.Request.Context())
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, surf)
	  })
  }
