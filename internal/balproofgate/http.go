// @FileName: http.go
// @Summary: 路由（挂在 /v1 下）：
// 路由（挂在 /v1 下）：
// POST /balproofgate/issue
// POST /balproofgate/recheck
// POST /balproofgate/consume
// POST /balproofgate/revoke
// GET /balproofgate/lease/:id
// GET /balproofgate/generation
// GET /balproofgate/drift/:id
// POST /balproofgate/drift/close
// GET /balproofgate/export/:id
// GET /balproofgate/stats
// 权限：查询 ActQueryRead；签发/复核/消费 ActQueryRead；吊销/收口 ActOpsAdmin。
// 主机占位沿用 query-gateway.kucoin.net，不改契约域名。
// @Author: Draven
// @Created: 2021.02.17

package balproofgate

import (
	"net/http"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 挂载路由。观察器挂钩 Runtime.Ledger（只读 Accounts）。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  secret := strings.TrimSpace(os.Getenv("BALPROOF_HMAC_SECRET"))
	   if secret == "" && rt != nil && rt.Cfg != nil {
		  secret = strings.TrimSpace(rt.Cfg.BalproofHMAC)
	   }
	   if secret == "" && rt != nil && rt.Mem {
		  secret = "mem-balproof-gate"
	  }
	  var obs LiveObserver
	  if rt != nil && rt.Ledger != nil {
		   obs = LedgerObserver{Led: rt.Ledger}
	   }
	  svc := NewService(secret, obs, NewMemStore())
	  registerRoutes(v1, svc, rt)
   }

  func registerRoutes(v1 *gin.RouterGroup, svc *Service, rt *app.Runtime) {
	  g := v1.Group("/balproofgate")
	   registerListRoutes(g, svc)
	  g.POST("/issue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body IssueReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   if rt != nil && rt.Cfg != nil {
			  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
			   body.UserID = uid
		  }
		   got, err := svc.Issue(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, got)
	   })
	   g.POST("/recheck", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body RecheckReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  lease, drift, err := svc.Recheck(c.Request.Context(), body)
		  if err != nil {
			  if drift != nil {
				  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "drift": drift, "lease": lease})
				  return
			   }
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"ok": true, "lease": lease})
	   })
	   g.POST("/consume", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body ConsumeReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   lease,
			  drift, err := svc.Consume(c.Request.Context(), body)
		   if err != nil {
			   if drift != nil {
				  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "drift": drift, "proof_lease_id": body.LeaseID})
				  return
			   }
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"ok": true, "lease": lease, "proof_lease_id": lease.ProofLeaseID})
	  })
	  g.POST("/revoke", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body RevokeReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  rev, lease, err := svc.Revoke(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"revocation": rev, "lease": lease})
	   })
	  g.GET("/lease/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  got, err := svc.GetLease(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, got)
	   })
	   g.GET("/generation", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   got, err := svc.GetGeneration(c.Request.Context(), c.Query("user_id"), c.Query("book"), c.Query("asset"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, got)
	   })
	   g.GET("/drift/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   got, err := svc.GetDrift(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, got)
	   })
	  g.POST("/drift/close", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   ID string `json:"id"`
			  Operator string `json:"operator"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.CloseDrift(c.Request.Context(), body.ID, body.Operator)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, got)
	   })
	   g.GET("/export/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   got, err := svc.Export(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, got)
	  })
	  g.GET("/audit", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"events": svc.AuditRecent(50)})
	  })
	   g.GET("/stats", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, svc.Stats())
	   })
   }
