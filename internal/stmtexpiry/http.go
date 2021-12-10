package stmtexpiry

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

// RegisterHTTP 注册 HTTP 路由到 gin RouterGroup。
   func RegisterHTTP(v1 *gin.RouterGroup, rt *app.Runtime) {
	  store := NewMemStore()
	   svc := NewService(store,
		   rt.Ledger, ServiceConfig{}, rt.Log)
	  v1.POST("/stmt-export/issue", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"),
			  callerid.FromRuntime(rt.Mem,
				   rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  uid, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  var body IssueExportReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   body.UserID = uid
		  if body.WindowEnd.IsZero() {
			   body.WindowEnd = time.Now().UTC()
		  }
		  if body.WindowStart.IsZero() {
			  body.WindowStart = body.WindowEnd.Add(-24 * time.Hour)
		   }
		   resp,
			   err := svc.IssueExport(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, resp)
	   })
	   v1.POST("/stmt-export/revoke", func(c *gin.Context) {
		  var body RevokeExportReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := svc.RevokeExport(c.Request.Context(), body); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/stmt-export/confirm-drift", func(c *gin.Context) {
		   var body ConfirmDriftReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.ConfirmDrift(c.Request.Context(), body); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true})
	   })
	   v1.GET("/stmt-export/verify", func(c *gin.Context) {
		   leaseID := c.Query("lease_id")
		  if leaseID == "" {
			  c.JSON(400, gin.H{"error": "lease_id required"})
			   return
		   }
		  if err := svc.VerifyExport(c.Request.Context(), leaseID); err != nil {
			   c.JSON(409, gin.H{"error": err.Error(), "valid": false})
			   return
		   }
		   c.JSON(200, gin.H{"valid": true})
	  })
	   v1.GET("/stmt-export/lease", func(c *gin.Context) {
		  leaseID := c.Query("lease_id")
		   if leaseID == "" {
			   c.JSON(400, gin.H{"error": "lease_id required"})
			   return
		  }
		  lease,
			   err := svc.GetLease(c.Request.Context(), leaseID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, lease)
	   })
	   v1.GET("/stmt-export/proof", func(c *gin.Context) {
		  leaseID := c.Query("lease_id")
		   if leaseID == "" {
			   c.JSON(400, gin.H{"error": "lease_id required"})
			   return
		   }
		   proof, err := svc.GetProof(c.Request.Context(), leaseID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, proof)
	  })
	  v1.GET("/stmt-export/active-leases", func(c *gin.Context) {
		   rawUID, err := callerid.Bind(c, c.Query("user_id"),
			   callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   uid, err := validate.UserID(rawUID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   leases, err := svc.ListActiveLeases(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"leases": leases, "count": len(leases)})
	  })
	   v1.GET("/stmt-export/drifts", func(c *gin.Context) {
		  leaseID := c.Query("lease_id")
		   if leaseID == "" {
			   c.JSON(400, gin.H{"error": "lease_id required"})
			   return
		  }
		  drifts, err := svc.ListDrifts(c.Request.Context(), leaseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"drifts": drifts, "count": len(drifts)})
	   })
	   v1.GET("/stmt-export/revocations", func(c *gin.Context) {
		  leaseID := c.Query("lease_id")
		   if leaseID == "" {
			   c.JSON(400, gin.H{"error": "lease_id required"})
			   return
		   }
		  revocations, err := svc.ListRevocations(c.Request.Context(), leaseID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"revocations": revocations, "count": len(revocations)})
	  })
  }
