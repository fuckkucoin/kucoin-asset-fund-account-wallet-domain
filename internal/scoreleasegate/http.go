package scoreleasegate

import (
	"net/http"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/riskscore"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   secret := strings.TrimSpace(os.Getenv("SCORELEASE_GATE_HMAC"))
	  if secret == "" && rt != nil && rt.Mem {
		  secret = strings.TrimSpace(os.Getenv("SCORELEASE_GATE_HMAC_MEM"))
	  }
	   svc := NewService(secret, RiskscoreEval{Svc: riskscore.New()}, NewMemStore())
	   registerRoutes(v1, svc)
  }

  func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	  g := v1.Group("/scoreleasegate")
	  registerListRoutes(g, svc)
	  g.POST("/eval", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body IssueReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got,
			  err := svc.EvalWithLease(c.Request.Context(), body)
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
				   "host":  "kyt.kucoin.net",
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
		  var body SetPackReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  gen, n, err := svc.SetPack(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	  })
	  g.POST("/mode", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body SetModeReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   gen, n, err := svc.SetMode(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"generation": gen, "revoked": n})
	  })
	   g.POST("/window", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body BumpWindowReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  gen, n, err := svc.BumpWindow(c.Request.Context(), body)
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
	   g.POST("/revoke", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body RevokeReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.Revoke(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, got)
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
		   got, err := svc.Export(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, got)
	  })
	   g.GET("/stats", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"stats": svc.Stats(), "host": "kyt.kucoin.net"})
	  })
   }
