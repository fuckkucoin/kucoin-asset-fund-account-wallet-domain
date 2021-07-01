// @FileName: travel_rails.go
// @Summary: 资金页旅行规则读面：门槛快照与待提交 outbound 列表（本进程）。
// @Author: Kinsley
// @Created: 2021.06.30

package gateway

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"github.com/gin-gonic/gin"
)

   func registerTravelRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	  opt := callerid.FromRuntime(false, "", "", "")
	   if rt != nil && rt.Cfg != nil {
		   opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	   }
	   gate := travelvasp.GateForRuntime(rt)
	  v1.GET("/assets/travel/policy", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  book := travelvasp.ThresholdBookFrom(rt)
		   c.JSON(200, gin.H{
			  "user_id":           uid,
			  "default_threshold": book.DefaultMin(),
			   "corridors": book.List(),
			   "usd_rates": book.ListUsdRates(),
			   "host":      config.HostKYT,
		   })
	  })
	  v1.GET("/assets/travel/requirements", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  view, err := gate.Requirements(travelvasp.RequirementsReq{
			   Asset: c.Query("currency"), Amount: c.Query("amount"),
			  BeneficiaryVASP: c.Query("beneficiaryVasp"),
		   })
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "requirements": view, "host": config.HostKYT})
	  })
	  v1.GET("/assets/travel/outbound/:id", func(c *gin.Context) {
		   _, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   msg, err := gate.GetOutbound(strings.TrimSpace(c.Param("id")))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"message": msg, "host": config.HostKYT})
	   })
   }
