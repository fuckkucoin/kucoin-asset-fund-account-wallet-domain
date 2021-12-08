package sweeprun

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	  if rt != nil {
		   svc.BindLedger(rt.Ledger)
	  }
	  v1.POST("/sweep/enqueue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body EnqueueReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.Enqueue(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	  v1.POST("/sweep/start", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   ID string `json:"id"`
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.Start(body.ID, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  v1.POST("/sweep/fill", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   ID     string `json:"id"`
			  Amount string `json:"amount"`
			  Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.Fill(c.Request.Context(), body.ID, body.Amount, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/sweep/resume", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   ID    string `json:"id"`
			  Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.Resume(body.ID, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	   v1.POST("/sweep/pause", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   ID    string `json:"id"`
			   Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Pause(body.ID, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/sweep/budget", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Chain string `json:"chain"`
			   Asset string `json:"asset"`
			   Cap   string `json:"cap"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if err := svc.SetBudget(body.Chain, body.Asset, body.Cap); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"ok": true})
	  })
	  v1.GET("/sweep/jobs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"jobs": svc.List(c.Query("chain"), 50), "host": "custody-boundary-lab.kucoin.net"})
	  })
	  v1.POST("/sweep/plans", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Actor string `json:"actor"`
			   Plans []Plan `json:"plans"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.EnqueuePlans(body.Actor, body.Plans)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"jobs": got})
	   })
	  v1.GET("/sweep/resume-queue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"jobs": svc.ScanResume(50)})
	  })
	   mountProof(v1, rt)
  }
