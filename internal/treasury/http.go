package treasury

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New(nil, nil)
	   v1.GET("/treasury/bands", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"bands": svc.Catalog().List(), "host": "custody-boundary-lab.kucoin.net"})
	  })
	   v1.POST("/treasury/plan", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   plans, err := svc.Plan(c.Request.Context(), rt.Store)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"plans": plans})
	  })
	  v1.POST("/treasury/alerts/scan", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  rows, err := svc.ScanAlerts(c.Request.Context(), rt.Store)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"alerts": rows})
	  })
	   v1.POST("/treasury/cold/confirm", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Chain string `json:"chain"`
			   TxHash string `json:"tx_hash"`
			   Asset string `json:"asset"`
			   Amount string `json:"amount"`
			  Confirms int `json:"confirmations"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if _, err := validate.TxHash(body.TxHash); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   rec, err := svc.Vault().ConfirmReceive(body.Chain, body.TxHash, body.Asset, body.Amount, body.Confirms)
		   if err != nil && err != ErrConfirm {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, rec)
	  })
	  v1.GET("/treasury/alerts", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"alerts": svc.Alerts().Recent(50)})
	   })
	   v1.GET("/treasury/cold/pending", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"pending": svc.Vault().Pending()})
	   })
	  v1.POST("/treasury/imbalance", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  rows,
			   err := svc.Imbalance(c.Request.Context(), rt.Store)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"imbalance": rows})
	  })
	  v1.POST("/treasury/exec", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   plans, err := svc.Plan(c.Request.Context(), rt.Store)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   rows, err := svc.RecordPlans(plans)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"exec": rows, "host": "custody-boundary-lab.kucoin.net"})
	  })
	   v1.GET("/treasury/exec", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"open": svc.Exec().Open(50)})
	  })
	   registerSafety(v1, rt, svc)
  }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  gate := fundsafety.New()
	  if rt != nil {
		  gate.BindLedger(rt.Ledger)
	  }
	   v1.POST("/treasury/refuse-negative", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Amount string `json:"amount"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if err := gate.RefuseNegative(body.Amount); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"ok": true, "host": "custody-boundary-lab.kucoin.net"})
	  })
	   v1.POST("/treasury/guard-write", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Split bool `json:"split"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  p := ha.NewPair("treasury", "a", "b", ha.NewMemBackend(), 0)
		  if _, err := p.Campaign(c.Request.Context()); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   if body.Split {
			  p.MarkSplit()
		  }
		  err := p.GuardWrite(func() error { return nil })
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true})
	  })
	  _ = svc
   }
