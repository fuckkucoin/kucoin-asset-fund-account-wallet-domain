package hotbound

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	  if rt != nil {
		  svc.BindStore(rt.Store)
	   }
	  v1.GET("/hotbound/observe", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   got, err := svc.Observe(c.Request.Context())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"snapshots": got, "host": "custody-boundary-lab.kucoin.net", "region": persist.DSNFor(persist.RegionSG)})
	   })
	  v1.POST("/hotbound/sweep", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body SweepReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.SweepToCold(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/hotbound/presubmit", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Asset string `json:"asset"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.PreSubmit(c.Request.Context(), body.Asset); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true, "read_only": true, "asset": body.Asset})
	  })
	   v1.POST("/hotbound/suggest", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   got,
			  err := svc.SuggestPlans(c.Request.Context())
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"plans": got, "execute": false})
	  })
	  v1.GET("/hotbound/recent", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"sweeps": svc.List(50), "host": "custody-boundary-lab.kucoin.net"})
	   })
	   mountReadGate(v1, svc)
  }
