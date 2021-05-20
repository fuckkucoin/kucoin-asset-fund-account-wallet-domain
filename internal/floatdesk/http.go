package floatdesk

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   _ = rt
	   v1.GET("/float/targets", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"targets": svc.Catalog().List(), "host": "custody-boundary-lab.kucoin.net"})
	   })
	   v1.POST("/float/targets", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body Target
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := svc.Catalog().Put(body); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, body)
	  })
	   v1.POST("/float/samples", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body Sample
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := svc.Record(body); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/float/forecast", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Chain string `json:"chain"`
			   Asset    string `json:"asset"`
			   HotAvail string `json:"hot_available"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Outlook(body.Chain, body.Asset, body.HotAvail, time.Hour)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	   v1.GET("/float/gas", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"tanks": svc.Tanks().List()})
	  })
	   v1.POST("/float/gas/refill", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  rows, err := svc.GasRefill()
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"refill": rows})
	  })
	  v1.POST("/float/halt", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Resume bool   `json:"resume"`
			   Chain  string `json:"chain"`
			   Asset  string `json:"asset"`
			  All    bool   `json:"all"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   switch {
		   case body.All && body.Resume:
			  svc.Halt().ResumeAll()
		  case body.All:
			  svc.Halt().HaltAll()
		  case body.Chain != "" && body.Resume:
			  svc.Halt().ResumeChain(body.Chain)
		   case body.Chain != "":
			   if err := svc.Halt().HaltChain(body.Chain); err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
		   case body.Asset != "" && body.Resume:
			  svc.Halt().ResumeAsset(body.Asset)
		   case body.Asset != "":
			  if err := svc.Halt().HaltAsset(body.Asset); err != nil {
				   mw.WriteProblem(c, err)
				   return
			   }
		   }
		   c.JSON(200, gin.H{"halt": svc.Halt().Snapshot()})
	  })
	  v1.POST("/float/fill", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  plans, err := svc.FillPlans(nil, time.Hour)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"plans": plans})
	  })
	  v1.POST("/float/egress/check", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Chain string `json:"chain"`
			   Asset    string `json:"asset"`
			  HotAvail string `json:"hot_available"`
			  FromTank bool `json:"from_tank"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  var ev InstantEgressEvent
		   var err error
		  if body.FromTank {
			  ev, err = svc.CheckEgressFromTanks(body.Chain, body.Asset)
		   } else {
			  ev, err = svc.CheckEgress(body.Chain, body.Asset, body.HotAvail)
		  }
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, ev)
	   })
	   v1.GET("/float/halt", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"halt": svc.Halt().Snapshot()})
	  })
  }
