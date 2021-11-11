// 路由（挂在 /v1 下）：
// POST /signpipegate/arm
// POST /signpipegate/enter
// POST /signpipegate/seal
// POST /signpipegate/fault
// POST /signpipegate/abort
// POST /signpipegate/fault/close
// POST /signpipegate/closeout/close
// GET /signpipegate/lease/:id
// GET /signpipegate/generation
// GET /signpipegate/artifact
// GET /signpipegate/export/:id
// GET /signpipegate/stats
// 权限：查询 ActQueryRead；注入/中止/收口 ActOpsAdmin。
// 主机占位沿用 query-gateway.kucoin.net，不改契约域名。

// Author: Ronan
// Date: 2021.11.09

package signpipegate

import (
	"net/http"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   secret := strings.TrimSpace(os.Getenv("SIGNPIPE_GATE_HMAC"))
	   if secret == "" && rt != nil && rt.Mem {
		  secret = "mem-signpipe-gate"
	   }
	  var obs JobObserver
	  if rt != nil {
		  obs = PipeObserver{Pipe: signpipe.New(DefaultLeaseTTL)}
	   }
	   store, err := BindStore(rt)
	   if err != nil {
		  if rt != nil {
			  rt.Log.Fatal().Err(err).Msg("signpipegate register store")
		  }
		  panic(err)
	   }
	  svc := NewService(secret,
		   obs, store)
	   registerRoutes(v1, svc)
  }

  func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	  g := v1.Group("/signpipegate")
	  registerListRoutes(g, svc)
	  g.POST("/arm", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body ArmReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.ArmRetry(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, got)
	  })
	   g.POST("/enter", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   LeaseID string `json:"lease_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Enter(c.Request.Context(), body.LeaseID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, got)
	   })
	  g.POST("/seal", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body SealReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  lease,
			  art, err := svc.Seal(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"lease": lease, "artifact": art})
	   })
	  g.POST("/fault", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body InjectReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got,
			   err := svc.InjectFault(c.Request.Context(), body)
		   if got != nil {
			  c.JSON(http.StatusOK, gin.H{"fault": got, "error": errString(err)})
			   return
		  }
		  mw.WriteProblem(c, err)
	   })
	  g.POST("/abort", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body AbortReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.Abort(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, got)
	   })
	   g.POST("/fault/close", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body FaultCloseReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.CloseFault(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, got)
	   })
	  g.POST("/closeout/close", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body CloseoutCloseReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.CloseProof(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, got)
	  })
   }

  func errString(err error) string {
	   if err == nil {
		   return ""
	   }
	   return err.Error()
   }
