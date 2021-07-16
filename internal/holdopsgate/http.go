package holdopsgate

import (
	"net/http"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  secret := strings.TrimSpace(os.Getenv("HOLDOPS_GATE_HMAC"))
	  if secret == "" && rt != nil && rt.Mem {
		   secret = strings.TrimSpace(os.Getenv("HOLDOPS_GATE_HMAC_MEM"))
	   }
	   store, err := BindStore(rt)
	   if err != nil {
		  if rt != nil {
			   rt.Log.Fatal().Err(err).Msg("holdopsgate register store")
		  }
		   panic(err)
	  }
	   svc := NewService(secret, store)
	   registerRoutes(v1, svc)
   }

  func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	  g := v1.Group("/holdopsgate")
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
		   got, err := svc.Issue(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, got)
	   })
	  g.POST("/exec", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body ExecReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  lease, proof, err := svc.Exec(c.Request.Context(), body)
		   if err != nil {
			   c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "lease": lease, "proof": proof})
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"lease": lease, "proof": proof})
	  })
	  g.POST("/place", typedExec(svc, OpsPlace))
	   g.POST("/release", typedExec(svc, OpsRelease))
	  g.POST("/ack", typedExec(svc, OpsGapAck))
	   g.POST("/close", typedExec(svc, OpsGapClose))
	  g.POST("/bump", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body BumpReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.Bump(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, got)
	   })
	   g.POST("/expire", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body ExpireReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  n,
			   err := svc.ExpireSweep(c.Request.Context(), body.Limit)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"expired": n})
	   })
	  g.POST("/four-eye", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body FourEyeReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.FourEye(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, got)
	  })
   }

  func typedExec(svc *Service, kind OpsKind) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body ExecReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  body.Kind = kind
		  lease, proof, err := svc.Exec(c.Request.Context(), body)
		  if err != nil {
			   c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "lease": lease, "proof": proof})
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"lease": lease, "proof": proof})
	   }
   }
