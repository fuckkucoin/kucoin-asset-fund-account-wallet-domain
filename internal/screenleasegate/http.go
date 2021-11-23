package screenleasegate

import (
	"net/http"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/riskgate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   secret := strings.TrimSpace(os.Getenv("SCREENLEASE_GATE_HMAC"))
	  if secret == "" && rt != nil && rt.Mem {
		  secret = "mem-screen-lease-gate"
	  }
	   if secret == "" {
		   secret = "screenleasegate-local-hmac"
	  }
	  store, err := BindStore(rt)
	  if err != nil {
		  if rt != nil {
			  rt.Log.Fatal().Err(err).Msg("screenleasegate register store")
		  }
		   panic(err)
	  }
	   svc := NewService(secret, RiskgateScreener{Svc: riskgate.New()}, store)
	   registerRoutes(v1, svc)
   }

  func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	   g := v1.Group("/screenleasegate")
	   registerListRoutes(g, svc)
	   g.POST("/issue", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
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
	  g.POST("/submit", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body SubmitReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   lease, proof, err := svc.Submit(c.Request.Context(), body)
		  if err != nil {
			  c.JSON(http.StatusConflict, gin.H{
				   "error": err.Error(),
				  "lease": lease,
				   "proof": proof,
				  "host": "kyt.kucoin.net",
			   })
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"lease": lease, "host": "kyt.kucoin.net"})
	   })
	   g.POST("/halt", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body SetHaltReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  gen, n, err := svc.SetHalt(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	   })
	   g.POST("/pack", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body FlipPackReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  gen,
			   n, err := svc.FlipPack(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	  })
	  g.POST("/expire", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   n, err := svc.ExpireSweep(c.Request.Context())
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
		  p,
			  err := svc.FourEye(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, p)
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
	  g.GET("/proof/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  got, err := svc.GetProof(c.Request.Context(), c.Param("id"))
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
		   got, err := svc.Generation(c.Request.Context())
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
		   doc, err := ExportProofBundle(c.Request.Context(), svc, c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, doc)
	  })
	  g.GET("/stats", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"stats": svc.Stats(), "host": "kyt.kucoin.net"})
	  })
  }
