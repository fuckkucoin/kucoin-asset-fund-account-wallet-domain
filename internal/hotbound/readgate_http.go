package hotbound

import (
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func mountReadGate(v1 *gin.RouterGroup, svc *Service) {
	   v1.POST("/hotbound/read-gate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Asset string `json:"asset"`
			  HotLow  string `json:"hot_low"`
			   HotHigh string `json:"hot_high"`
			   WarmLow string `json:"warm_low"`
			  WarmHigh string `json:"warm_high"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   before := len(svc.List(100))
		  got, err := svc.ReadGate(c.Request.Context(), body.Asset, hotinv.GateBand{
			   HotLow:  body.HotLow,
			  HotHigh: body.HotHigh, WarmLow: body.WarmLow, WarmHigh: body.WarmHigh,
		  })
		   if err != nil && got.Code == "" {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{
			   "verdict": got,
			   "error": errString(err),
			  "sweep_count":  len(svc.List(100)),
			   "sweep_before": before,
			  "wrote_sweep": false,
		  })
	  })
  }

  func errString(err error) string {
	  if err == nil {
		  return ""
	   }
	  return err.Error()
  }
