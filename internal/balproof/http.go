package balproof

import (
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  secret := strings.TrimSpace(os.Getenv("BALPROOF_HMAC_SECRET"))
	  if secret == "" && rt != nil && rt.Cfg != nil {
		   secret = strings.TrimSpace(rt.Cfg.BalproofHMAC)
	  }
	   if secret == "" && rt != nil && rt.Mem {
		   secret = "mem-balproof"
		  if rt.Cfg != nil && rt.Cfg.SeedUserID != "" {
			  secret = "mem-balproof:" + rt.Cfg.SeedUserID
		  }
	  }
	   svc := New(secret)
	  if rt != nil {
		   svc.BindLedger(rt.Ledger)
	   }
	   v1.POST("/balproof/issue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body IssueReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   body.UserID = uid
		  got, err := svc.Issue(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.Header("Deprecation", "true")
		  c.Header("X-Solvency-Class", EngineeringClass)
		  c.JSON(200, got)
	  })
	  v1.POST("/balproof/verify", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body VerifyReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := svc.Check(body); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.Header("Deprecation", "true")
		   c.Header("X-Solvency-Class", EngineeringClass)
		   c.JSON(200, gin.H{
			   "ok": true, "host": config.HostQueryGateway,
			  "deprecated": true, "solvency": false, "class": EngineeringClass,
			  "disclaimer": Disclaimer,
		  })
	  })
	   v1.GET("/balproof/used", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.Header("Deprecation", "true")
		  c.JSON(200, gin.H{
			   "nonces": svc.Used(50), "disclaimer": Disclaimer,
			  "deprecated": true, "solvency": false, "class": EngineeringClass,
		   })
	   })
  }
